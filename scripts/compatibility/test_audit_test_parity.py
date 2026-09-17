from __future__ import annotations

import importlib.util
import json
import os
import pathlib
import tempfile
import unittest


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


if __name__ == "__main__":
    unittest.main()
