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
	assertNoStalePineRuntimePerformanceGate(t, root)
	assertReleaseFrontendAssetsAreAudited(t, root)
	assertReleasePineWorkerAssetsAreAudited(t, root)
	assertPinetsReleaseRequiresInstalledPackage(t, root)
	assertBunSEAPackagingIsDocumented(t, root)
	assertPineTSWorkerUsesStaticImportForBunSEA(t, root)
	assertCIExercisesPineTSWorker(t, root)
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

func assertNoStalePineRuntimePerformanceGate(t *testing.T, root string) {
	t.Helper()
	rel := ".github/workflows/backtest-performance-gate.yml"
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", rel, err)
	}
	text := string(data)
	for _, stale := range []string{
		"pkg/strategy/pineruntime",
		"BenchmarkPineRuntime",
		"BenchmarkRunExecutesPineGoldenMatrix",
	} {
		if strings.Contains(text, stale) {
			t.Fatalf("%s still references removed Go Pine performance gate %q", rel, stale)
		}
	}
	if !strings.Contains(text, "BenchmarkCheckPerformanceGate") {
		t.Fatalf("%s does not run the PineTS worker performance gate", rel)
	}
}

func assertReleaseFrontendAssetsAreAudited(t *testing.T, root string) {
	t.Helper()
	rel := "internal/frontendassets/release_test.go"
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", rel, err)
	}
	text := string(data)
	for _, required := range []string{
		"TestFileSystemDoesNotEmbedRemovedGoPineRuntimeReferences",
		"pkg/strategy/pineruntime",
		"BenchmarkPineRuntime",
		"BenchmarkRunExecutesPineGoldenMatrix",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("%s does not audit release frontend assets for %q", rel, required)
		}
	}
}

func assertReleasePineWorkerAssetsAreAudited(t *testing.T, root string) {
	t.Helper()
	requiredByFile := map[string][]string{
		"internal/pineworkerassets/assets_dev_test.go": {
			"TestSelectForPlatformReturnsUnavailableWhenAssetMissing",
			"!release_assets",
		},
		"internal/pineworkerassets/assets_release_test.go": {
			"TestSelectForPlatformReturnsEmbeddedAssetWhenStaged",
			"release_assets",
			"SHA256",
		},
	}
	for rel, requiredValues := range requiredByFile {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", rel, err)
		}
		text := string(data)
		for _, required := range requiredValues {
			if !strings.Contains(text, required) {
				t.Fatalf("%s does not audit Pine worker release assets for %q", rel, required)
			}
		}
	}
}

func assertPinetsReleaseRequiresInstalledPackage(t *testing.T, root string) {
	t.Helper()
	requiredByFile := map[string][]string{
		"scripts/build-pineworker-assets.mjs": {
			"PineTS worker asset build is blocked until the pinets package is installed",
			"checkPinetsPackageAndLicense",
		},
		"scripts/build-pineworker-assets.test.mjs": {
			"pinets package license: AGPL-3.0-only",
			"AGPL-3.0-only",
			"DRY RUN bun build",
		},
		"scripts/check-pinets-release.mjs": {
			"checkPinetsPackageAndLicense",
			"check:pinets-compliance",
			"test:web",
			"typecheck:web",
			"diff",
			"--check",
			"dist/trading-engine",
			"build",
			"release_assets",
			"-o",
			"prepareReleaseArtifactPath",
			"verifyReleaseArtifact",
			"release artifact is missing or empty",
		},
		"scripts/lib/pinets-package.mjs": {
			"checkPinetsPackageAndLicense",
			"Checking pinets package",
			"pinets package license:",
		},
		"scripts/check-pinets-release.test.mjs": {
			"pinets package license: AGPL-3.0-only",
			"AGPL-3.0-only",
			"check:pinets-compliance",
			"test:web",
			"typecheck:web",
			"git diff --check",
			"JFTRADE_PINETS_RELEASE_OUT",
			"go build -tags release_assets -o",
			"JFTRADE_PINETS_RELEASE_STUB_SKIP_ARTIFACT",
			"release artifact is missing or empty",
		},
		"docs/troubleshooting/pinets-worker-release.md": {
			"商业 PineTS 授权计划已取消",
			"AGPL-3.0-only",
			"发布产物必须存在、非空且可执行",
		},
		"docs/legal/third-party-notices.md": {
			"runtime=pine-pinets",
			"workers/pineworker",
			"scripts/build-pineworker-assets.mjs",
			"scripts/check-pinets-release.mjs",
			"network users",
			"corresponding source",
		},
		"scripts/check-pinets-compliance.mjs": {
			"AGPL-3.0-only",
			"runtime=pine-pinets",
			"workers/pineworker",
			"shadow-only",
		},
		"package.json": {
			"check:pinets-compliance",
		},
		"workers/pineworker/package.json": {
			"\"pinets\": \"^0.9.26\"",
		},
	}
	for rel, requiredValues := range requiredByFile {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", rel, err)
		}
		text := string(data)
		for _, required := range requiredValues {
			if !strings.Contains(text, required) {
				t.Fatalf("%s does not gate PineTS release on installed package evidence %q", rel, required)
			}
		}
	}
}

func assertBunSEAPackagingIsDocumented(t *testing.T, root string) {
	t.Helper()
	requiredByFile := map[string][]string{
		"docs/pinets-hardcut-migration.md": {
			"Bun SEA",
			"bun build --compile",
			"release_assets",
			"trading-engine",
		},
		"docs/troubleshooting/pinets-worker-release.md": {
			"Bun SEA",
			"bun build --compile",
			"release_assets",
			"trading-engine",
		},
		"scripts/build-pineworker-assets.mjs": {
			"--compile",
		},
		"scripts/build-pineworker-dev.sh": {
			"build-pineworker-dev.mjs",
		},
		"scripts/build-pineworker-dev.mjs": {
			"--compile",
			"JFTRADE_PINEWORKER_DEV_OUT_DIR",
			"JFTRADE_PINEWORKER_DEV_ENV_FILE",
			"checkPinetsPackageAndLicense",
		},
		"scripts/dev-api-pineworker.mjs": {
			"buildDevWorker",
			"JFTRADE_PINEWORKER_BINARY",
			"cmd/jftrade-api",
		},
		"package.json": {
			"build:pineworker:dev",
			"dev:api:pineworker",
			"test:pineworker-dev-build",
		},
		".vscode/tasks.json": {
			"build:pineworker:dev",
			"JFTRADE_PINEWORKER_DEV_ENV_FILE",
			"var/pineworker/vscode.env",
		},
		".vscode/launch.json": {
			"Dev Backend with PineTS Worker",
			"preLaunchTask",
			"build:pineworker:dev",
			"envFile",
			"var/pineworker/vscode.env",
			"JFTRADE_PINEWORKER_WORKERS",
		},
		"internal/app/apiserver/servercore/pineworker_runtime.go": {
			"npm run dev:api:pineworker",
			"JFTRADE_PINEWORKER_BINARY",
			"/absolute/path/to/worker",
			"settingsfile.DefaultPineWorkerSettings",
			"WorkerLimit",
			"envIntInRange",
			"1000",
			"newLazyPineWorkerRunner",
			"defaultPineWorkerIdleTimeout",
		},
		"pkg/jftsettings/types.go": {
			"PineWorkerSettings",
			"workerLimit",
			"workerCount",
		},
		"internal/store/settingsfile/store.go": {
			"DefaultPineWorkerSettings",
			"runtime.NumCPU",
			"NormalizePineWorkerSettings",
			"1000",
		},
		"internal/api/settings/routes.go": {
			"/pine-worker",
			"handlePineWorkerSettings",
			"handleSavePineWorkerSettings",
		},
		"apps/web/src/components/SettingsPineWorkerSection.vue": {
			"workerLimit",
			"MAX_WORKER_LIMIT = 1000",
			"/api/v1/settings/pine-worker",
		},
	}
	for rel, requiredValues := range requiredByFile {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", rel, err)
		}
		text := string(data)
		for _, required := range requiredValues {
			if !strings.Contains(text, required) {
				t.Fatalf("%s does not document Bun SEA packaging requirement %q", rel, required)
			}
		}
	}
}

func assertPineTSWorkerUsesStaticImportForBunSEA(t *testing.T, root string) {
	t.Helper()
	rel := "workers/pineworker/src/pinetsExecutor.ts"
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", rel, err)
	}
	text := string(data)
	if !strings.Contains(text, `import { PineTS } from "pinets"`) {
		t.Fatalf("%s must statically import pinets for Bun SEA packaging", rel)
	}
	if strings.Contains(text, `import("pinets")`) || strings.Contains(text, "dynamicImport") {
		t.Fatalf("%s must not use dynamic pinets import in Bun SEA packaging", rel)
	}
}

func assertCIExercisesPineTSWorker(t *testing.T, root string) {
	t.Helper()
	rel := ".github/workflows/ci.yml"
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", rel, err)
	}
	text := string(data)
	for _, required := range []string{
		"oven-sh/setup-bun",
		"npm run test:pineworker",
		"npm run typecheck:pineworker",
		"npm run build:frontend-assets",
		"go test -tags release_assets ./internal/frontendassets -run TestFileSystem",
		"npm run test:pinets-release-check",
		"npm run check:pinets-compliance",
		"npm run test:pinets-shadow-corpus",
		"JFTRADE_PINETS_SHADOW_REPORT_PATH",
		"actions/upload-artifact",
		"npm run test:pineworker-asset-build",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("%s does not exercise PineTS worker gate %q", rel, required)
		}
	}
}
