from __future__ import annotations

import importlib.util
import json
import os
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock


SCRIPT_PATH = pathlib.Path(__file__).with_name("audit_test_parity.py")
SPEC = importlib.util.spec_from_file_location("audit_test_parity", SCRIPT_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {SCRIPT_PATH}")
AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(AUDIT)


class RustReferenceResolutionTest(unittest.TestCase):
    """The audit must only approve mappings whose Rust entry really exists."""

    def setUp(self) -> None:
        self._original_cwd = os.getcwd()
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = pathlib.Path(self._tmp.name)
        (self.root / "crates" / "jftrade-demo" / "src").mkdir(parents=True)
        os.chdir(self.root)

    def tearDown(self) -> None:
        os.chdir(self._original_cwd)

    def write(self, relative: str, content: str) -> None:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def resolve(self, path: str, module_path: str) -> bool:
        tests_by_file, redirects = AUDIT._rust_test_index()
        return AUDIT._resolve_rust_test_reference(tests_by_file, redirects, path, module_path)

    def test_plain_test_in_named_file_resolves(self) -> None:
        self.write("crates/jftrade-demo/src/lib.rs", "#[test]\nfn behavior_is_kept() {}\n")
        self.assertTrue(self.resolve("crates/jftrade-demo/src/lib.rs", "behavior_is_kept"))

    def test_comment_between_attribute_and_fn_still_resolves(self) -> None:
        # The workspace convention puts a `// Parity:` anchor between the
        # attribute and the function; the parser must not miss those tests.
        self.write(
            "crates/jftrade-demo/src/lib.rs",
            "#[test]\n// Parity: internal/api/routes_test.go:1 TestX\nfn anchored() {}\n",
        )
        self.assertTrue(self.resolve("crates/jftrade-demo/src/lib.rs", "anchored"))

    def test_extra_attributes_between_attribute_and_fn_resolve(self) -> None:
        self.write(
            "crates/jftrade-demo/src/lib.rs",
            "#[tokio::test]\n#[ignore]\nasync fn gated() {}\n",
        )
        self.assertTrue(self.resolve("crates/jftrade-demo/src/lib.rs", "gated"))

    def test_path_redirect_to_sibling_test_module_resolves(self) -> None:
        self.write(
            "crates/jftrade-demo/src/owner.rs",
            '#[cfg(test)]\n#[path = "owner_tests.rs"]\nmod tests;\n',
        )
        self.write("crates/jftrade-demo/src/owner_tests.rs", "#[test]\nfn nested_case() {}\n")
        self.assertTrue(self.resolve("crates/jftrade-demo/src/owner.rs", "tests::nested_case"))

    def test_sibling_tests_file_without_redirect_resolves(self) -> None:
        self.write("crates/jftrade-demo/src/owner.rs", "pub fn read() {}\n")
        self.write("crates/jftrade-demo/src/owner_tests.rs", "#[test]\nfn sibling_case() {}\n")
        self.assertTrue(self.resolve("crates/jftrade-demo/src/owner.rs", "sibling_case"))

    def test_missing_function_does_not_resolve(self) -> None:
        self.write("crates/jftrade-demo/src/lib.rs", "#[test]\nfn real_name() {}\n")
        self.assertFalse(self.resolve("crates/jftrade-demo/src/lib.rs", "renamed_away"))

    def test_production_symbol_is_not_a_test(self) -> None:
        # A helper the entry cites as context must not count as evidence.
        self.write("crates/jftrade-demo/src/lib.rs", "fn dispatch_request() {}\n")
        self.assertFalse(self.resolve("crates/jftrade-demo/src/lib.rs", "dispatch_request"))


class BrokenReferenceReportingTest(unittest.TestCase):
    """Approvals must fail the audit; acknowledged gaps only warn."""

    def setUp(self) -> None:
        self._original_cwd = os.getcwd()
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = pathlib.Path(self._tmp.name)
        (self.root / "crates" / "jftrade-demo" / "src").mkdir(parents=True)
        (self.root / "crates" / "jftrade-demo" / "src" / "lib.rs").write_text(
            "#[test]\nfn existing_case() {}\n", encoding="utf-8"
        )
        os.chdir(self.root)

    def tearDown(self) -> None:
        os.chdir(self._original_cwd)

    def mapping(self, evidence: str, rust_entry: str) -> dict:
        return {
            "go_test.go:1:TestX": {
                "status": "[x]" if evidence == "function_exact" else "[~]",
                "rust_entry": rust_entry,
                "conclusion": "conclusion",
                "command": "command",
                "evidence_type": evidence,
            }
        }

    def test_existing_reference_is_not_reported(self) -> None:
        approvals, acknowledged = AUDIT.unresolved_parity_references(
            self.mapping(
                "function_exact",
                "crates/jftrade-demo/src/lib.rs::existing_case",
            )
        )
        self.assertEqual([], approvals)
        self.assertEqual([], acknowledged)

    def test_stale_filename_is_reported_for_approval(self) -> None:
        approvals, _ = AUDIT.unresolved_parity_references(
            self.mapping(
                "function_exact",
                "crates/jftrade-demo/src/lib.rs::renamed_away",
            )
        )
        self.assertEqual(1, len(approvals))
        self.assertIn("renamed_away", approvals[0][1])

    def test_approval_without_any_reference_is_reported(self) -> None:
        approvals, _ = AUDIT.unresolved_parity_references(
            self.mapping("function_exact", "见 production 模块")
        )
        self.assertEqual(1, len(approvals))

    def test_partial_with_unresolvable_entry_warns_without_failing(self) -> None:
        # Partial rows acknowledge their own gap. A citation that no longer
        # names a test is worth surfacing, but it must never fail the audit.
        approvals, stale = AUDIT.unresolved_parity_references(
            self.mapping("partial", "crates/jftrade-demo/src/lib.rs::dispatch_request")
        )
        self.assertEqual([], approvals)
        self.assertEqual(1, len(stale))
        self.assertIn("dispatch_request", stale[0][1])

    def test_partial_without_any_reference_is_not_reported(self) -> None:
        # A partial row may explain its gap in prose only.
        approvals, stale = AUDIT.unresolved_parity_references(
            self.mapping("partial", "生产入口见 product_runtime.rs")
        )
        self.assertEqual([], approvals)
        self.assertEqual([], stale)

    def test_boundary_and_missing_rows_are_ignored(self) -> None:
        entries = {
            "a.go:1:TestA": {
                "status": "[~]", "rust_entry": "不适用：无对应入口",
                "conclusion": "", "command": "", "evidence_type": "boundary",
            },
            "b.go:2:TestB": {
                "status": "[~]", "rust_entry": "", "conclusion": "",
                "command": "", "evidence_type": "missing",
            },
        }
        approvals, acknowledged = AUDIT.unresolved_parity_references(entries)
        self.assertEqual([], approvals)
        self.assertEqual([], acknowledged)


class CrateQualifiedReferenceTest(unittest.TestCase):
    """Entries that name a test by crate path are checked the same way."""

    def setUp(self) -> None:
        self._original_cwd = os.getcwd()
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = pathlib.Path(self._tmp.name)
        (self.root / "crates" / "jftrade-demo" / "src").mkdir(parents=True)
        (self.root / "crates" / "jftrade-demo" / "src" / "lib.rs").write_text(
            "#[test]\nfn desktop_contract_case() {}\n", encoding="utf-8"
        )
        os.chdir(self.root)

    def tearDown(self) -> None:
        os.chdir(self._original_cwd)

    def mapping(self, rust_entry: str) -> dict:
        return {
            "go_test.go:1:TestX": {
                "status": "[x]",
                "rust_entry": rust_entry,
                "conclusion": "conclusion",
                "command": "command",
                "evidence_type": "function_exact",
            }
        }

    def test_existing_crate_qualified_test_resolves(self) -> None:
        approvals, _ = AUDIT.unresolved_parity_references(
            self.mapping("`jftrade-desktop::desktop_contracts::desktop_contract_case`")
        )
        self.assertEqual([], approvals)

    def test_unknown_crate_qualified_test_is_reported(self) -> None:
        approvals, _ = AUDIT.unresolved_parity_references(
            self.mapping("`jftrade-desktop::desktop_contracts::missing_case`")
        )
        self.assertEqual(1, len(approvals))


class CargoPackageCacheTest(unittest.TestCase):
    """Workspace package resolution must not re-run cargo metadata per mapping."""

    def setUp(self) -> None:
        AUDIT._cargo_package_names_cache = None
        self.addCleanup(setattr, AUDIT, "_cargo_package_names_cache", None)

    def cargo_result(self, returncode: int, stdout: str = "") -> subprocess.CompletedProcess:
        return subprocess.CompletedProcess(
            args=["cargo", "metadata"], returncode=returncode, stdout=stdout, stderr=""
        )

    def test_repeated_lookups_run_cargo_metadata_once(self) -> None:
        payload = json.dumps({"packages": [{"name": "jftrade-demo"}]})
        with mock.patch.object(
            AUDIT.subprocess, "run", return_value=self.cargo_result(0, payload)
        ) as run:
            self.assertTrue(AUDIT._is_known_cargo_package("jftrade-demo"))
            self.assertFalse(AUDIT._is_known_cargo_package("jftrade-missing"))
        self.assertEqual(1, run.call_count)

    def test_cargo_failure_falls_back_to_path_probe(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root / "crates" / "jftrade-demo").mkdir(parents=True)
            original = os.getcwd()
            os.chdir(root)
            try:
                with mock.patch.object(
                    AUDIT.subprocess, "run", return_value=self.cargo_result(1)
                ):
                    self.assertTrue(AUDIT._is_known_cargo_package("jftrade-demo"))
                    self.assertFalse(AUDIT._is_known_cargo_package("jftrade-missing"))
            finally:
                os.chdir(original)


class AssertionlessApprovalTest(unittest.TestCase):
    """Approvals citing assertion-free tests are the cheapest way to inflate coverage."""

    def setUp(self) -> None:
        self._original_cwd = os.getcwd()
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = pathlib.Path(self._tmp.name)
        (self.root / "crates" / "jftrade-demo" / "src" / "lib.rs").parent.mkdir(parents=True)
        (self.root / "crates" / "jftrade-demo" / "src" / "lib.rs").write_text(
            "#[test]\nfn empty_case() {}\n"
            "#[test]\nfn asserting_case() { assert_eq!(1, 1); }\n",
            encoding="utf-8",
        )
        os.chdir(self.root)

    def tearDown(self) -> None:
        os.chdir(self._original_cwd)

    def mapping(self, evidence: str, rust_entry: str) -> dict:
        return {
            "go_test.go:1:TestX": {
                "status": "[x]" if evidence == "function_exact" else "[~]",
                "rust_entry": rust_entry,
                "conclusion": "conclusion",
                "command": "command",
                "evidence_type": evidence,
            }
        }

    def test_approval_citing_assertion_free_test_is_listed(self) -> None:
        warnings = AUDIT.assertionless_approved_references(
            self.mapping(
                "function_exact",
                "crates/jftrade-demo/src/lib.rs::empty_case",
            )
        )
        self.assertEqual(1, len(warnings))

    def test_approval_citing_asserting_test_is_not_listed(self) -> None:
        warnings = AUDIT.assertionless_approved_references(
            self.mapping(
                "function_exact",
                "crates/jftrade-demo/src/lib.rs::asserting_case",
            )
        )
        self.assertEqual([], warnings)

    def test_partial_rows_are_not_checked(self) -> None:
        warnings = AUDIT.assertionless_approved_references(
            self.mapping("partial", "crates/jftrade-demo/src/lib.rs::empty_case")
        )
        self.assertEqual([], warnings)

    def test_unresolvable_approval_is_left_to_reference_check(self) -> None:
        # A reference that names no test at all already fails the audit via
        # unresolved_parity_references; the assertion heuristic stays silent.
        warnings = AUDIT.assertionless_approved_references(
            self.mapping("function_exact", "crates/jftrade-demo/src/lib.rs::ghost_case")
        )
        self.assertEqual([], warnings)


class UnanchoredApprovalTest(unittest.TestCase):
    """A claim must be written in code too, not only in the inventory.

    Reference existence alone cannot tell a genuine approval from an anchor
    that drifted onto the wrong test, so the two halves of the claim are
    cross-checked and the disagreement is reported for review.
    """

    def setUp(self) -> None:
        self._original_cwd = os.getcwd()
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = pathlib.Path(self._tmp.name)
        self.write(
            "crates/jftrade-demo/src/lib.rs",
            "#[test]\nfn proven_case() {}\n",
        )
        os.chdir(self.root)

    def tearDown(self) -> None:
        os.chdir(self._original_cwd)

    def write(self, relative: str, content: str) -> None:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def mapping(self, rust_entry: str, evidence: str = "function_exact") -> dict:
        return {
            "internal/api/routes_test.go:42:TestBehaviour": {
                "status": "[x]" if evidence == "function_exact" else "[~]",
                "rust_entry": rust_entry,
                "conclusion": "conclusion",
                "command": "command",
                "evidence_type": evidence,
            }
        }

    def anchors(self, *files: str) -> dict:
        return {
            ("internal/api/routes_test.go", 42): [
                {"rust_file": path, "rust_line": 1, "go_test": "TestBehaviour"}
                for path in files
            ]
        }

    def test_anchor_in_the_cited_file_is_not_reported(self) -> None:
        unanchored = AUDIT.unanchored_approvals(
            self.mapping("crates/jftrade-demo/src/lib.rs::proven_case"),
            self.anchors("crates/jftrade-demo/src/lib.rs"),
        )
        self.assertEqual([], unanchored)

    def test_missing_anchor_anywhere_is_reported(self) -> None:
        unanchored = AUDIT.unanchored_approvals(
            self.mapping("crates/jftrade-demo/src/lib.rs::proven_case"),
            {},
        )
        self.assertEqual(1, len(unanchored))
        self.assertIn("没有任何", unanchored[0][1])

    def test_anchor_on_a_different_file_is_reported_with_both_sides(self) -> None:
        unanchored = AUDIT.unanchored_approvals(
            self.mapping("crates/jftrade-demo/src/lib.rs::proven_case"),
            self.anchors("crates/elsewhere/src/lib.rs"),
        )
        self.assertEqual(1, len(unanchored))
        detail = unanchored[0][1]
        self.assertIn("crates/jftrade-demo/src/lib.rs::proven_case", detail)
        self.assertIn("crates/elsewhere/src/lib.rs", detail)

    def test_sibling_test_module_is_resolved_before_reporting(self) -> None:
        # The inventory may name the owner file while the test — and its
        # anchor — lives in the sibling `<stem>_tests.rs` module.
        self.write(
            "crates/jftrade-demo/src/sibling.rs",
            "#[cfg(test)]\n#[path = \"sibling_tests.rs\"]\nmod tests;\n",
        )
        self.write(
            "crates/jftrade-demo/src/sibling_tests.rs",
            "#[test]\nfn proven_case() {}\n",
        )
        unanchored = AUDIT.unanchored_approvals(
            self.mapping("crates/jftrade-demo/src/sibling.rs::tests::proven_case"),
            self.anchors("crates/jftrade-demo/src/sibling_tests.rs"),
        )
        self.assertEqual([], unanchored)

    def test_partial_and_boundary_rows_are_not_checked(self) -> None:
        for evidence in ("partial", "boundary", "missing"):
            with self.subTest(evidence=evidence):
                unanchored = AUDIT.unanchored_approvals(
                    self.mapping("crates/jftrade-demo/src/lib.rs::proven_case", evidence),
                    {},
                )
                self.assertEqual([], unanchored)

    def test_approval_without_a_reference_is_left_to_the_existence_check(self) -> None:
        unanchored = AUDIT.unanchored_approvals(
            self.mapping("未引用任何 Rust 测试"),
            {},
        )
        self.assertEqual([], unanchored)


class GoDomainClassificationTest(unittest.TestCase):
    """Migrated families must be counted in their own domain, never in "other".

    A domain matrix that files finished work under ``other`` understates the
    domains that actually did it, so each of these routes is pinned by name.
    """

    def test_exchange_calendar_belongs_to_backtest_calendar(self) -> None:
        self.assertEqual(
            "backtest_calendar",
            AUDIT.classify_go_domain("internal/exchangecalendar/builtin_test.go"),
        )

    def test_product_features_belong_to_marketdata_quotes(self) -> None:
        self.assertEqual(
            "marketdata_quotes",
            AUDIT.classify_go_domain("internal/productfeatures/service_test.go"),
        )

    def test_nested_calendar_prefix_beats_the_broader_market_prefix(self) -> None:
        # ``pkg/market`` is marketdata_quotes, but the calendar sub-package is
        # the exchange-calendar family and must win on prefix specificity.
        self.assertEqual(
            "marketdata_quotes",
            AUDIT.classify_go_domain("pkg/market/market_test.go"),
        )
        self.assertEqual(
            "backtest_calendar",
            AUDIT.classify_go_domain("pkg/market/calendar/builtin_test.go"),
        )

    def test_asset_packages_do_not_leak_into_business_domains(self) -> None:
        # These sibling directories only share a name prefix with a business
        # package; their bundle-selection tests are release tooling.
        marketdata_assets = os.path.join(
            "internal", "marketdataassets", "assets_test.go"
        )
        pineworker_assets = os.path.join(
            "internal", "pineworkerassets", "assets_test.go"
        )
        self.assertEqual(
            "other",
            AUDIT.classify_go_domain(marketdata_assets),
        )
        self.assertEqual(
            "other",
            AUDIT.classify_go_domain(pineworker_assets),
        )

    def test_unmapped_family_is_reported_as_other(self) -> None:
        self.assertEqual("other", AUDIT.classify_go_domain("pkg/bbgo/strategy_test.go"))

    def test_every_declared_prefix_matches_the_go_tree(self) -> None:
        # A prefix that names no real path can never route anything, which is
        # how completed ``internal/productfeatures`` rows silently fell through
        # to "other" for so long.
        result = subprocess.run(
            ["git", "ls-tree", "-r", "--name-only", "go"],
            capture_output=True, text=True,
        )
        if result.returncode != 0 or not result.stdout.strip():
            self.skipTest("the frozen 'go' baseline branch is not available here")
        paths = result.stdout.splitlines()
        for domain, _, prefixes, _ in AUDIT.DOMAIN_MAPPING:
            for prefix in prefixes:
                with self.subTest(domain=domain, prefix=prefix):
                    self.assertTrue(
                        any(path.startswith(prefix + "/") for path in paths),
                        f"{domain} declares {prefix!r}, which matches no Go path",
                    )


if __name__ == "__main__":
    unittest.main()
