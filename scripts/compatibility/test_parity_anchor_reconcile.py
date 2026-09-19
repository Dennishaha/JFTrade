from __future__ import annotations

import importlib.util
import json
import os
import pathlib
import tempfile
import unittest


SCRIPT_PATH = pathlib.Path(__file__).with_name("parity_anchor_reconcile.py")
SPEC = importlib.util.spec_from_file_location("parity_anchor_reconcile", SCRIPT_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {SCRIPT_PATH}")
RECONCILE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RECONCILE)


class AnchorCollectionTest(unittest.TestCase):
    """Anchors are only Go *test* references, in any of the supported shapes."""

    def setUp(self) -> None:
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = pathlib.Path(self._tmp.name)
        (self.root / "crates" / "demo" / "src").mkdir(parents=True)

    def collect(self, content: str) -> dict:
        path = self.root / "crates" / "demo" / "src" / "lib.rs"
        path.write_text(content, encoding="utf-8")
        relative = os.path.relpath(path, self.root)
        original = os.getcwd()
        os.chdir(self.root)
        try:
            return RECONCILE.collect_anchors([relative])
        finally:
            os.chdir(original)

    def test_anchor_with_revision_and_test_name(self) -> None:
        anchors = self.collect(
            "// Parity: go:452dea11:internal/api/routes_test.go:42 TestSomeBehaviour\n"
        )
        self.assertIn(("internal/api/routes_test.go", 42), anchors)
        self.assertEqual("TestSomeBehaviour", anchors[("internal/api/routes_test.go", 42)][0]["go_test"])

    def test_anchor_without_revision(self) -> None:
        anchors = self.collect(
            "// Parity: internal/api/routes_test.go:7 TestOtherBehaviour\n"
        )
        self.assertIn(("internal/api/routes_test.go", 7), anchors)

    def test_anchor_without_test_name(self) -> None:
        anchors = self.collect("/// Parity: go:452dea11:pkg/futu/exchange_test.go:120\n")
        self.assertIn(("pkg/futu/exchange_test.go", 120), anchors)
        self.assertIsNone(anchors[("pkg/futu/exchange_test.go", 120)][0]["go_test"])

    def test_production_file_reference_is_not_an_anchor(self) -> None:
        # `//! Parity: internal/marketdata/service.go::GetCandles` names
        # implementation, not a Go test, and must be ignored.
        anchors = self.collect("//! Parity: `internal/marketdata/service.go::GetCandles`\n")
        self.assertEqual({}, dict(anchors))

    def test_multiple_anchors_on_one_line_are_all_collected(self) -> None:
        anchors = self.collect(
            "// Parity: internal/a/x_test.go:1 TestA and internal/b/y_test.go:2 TestB\n"
        )
        self.assertIn(("internal/a/x_test.go", 1), anchors)
        self.assertIn(("internal/b/y_test.go", 2), anchors)


class ReconcileTest(unittest.TestCase):
    """Buckets separate 'code claims a mapping' from 'inventory admits a gap'."""

    def inventory(self, rows: dict) -> dict:
        return rows

    def test_unrecorded_when_anchor_lands_on_missing_row(self) -> None:
        inventory = self.inventory({
            "internal/api/routes_test.go:42:TestSomeBehaviour": {
                "status": "[~]",
                "rust_entry": "",
                "conclusion": "",
                "command": "",
                "evidence_type": "missing",
            }
        })
        anchors = {
            ("internal/api/routes_test.go", 42): [
                {"rust_file": "crates/x/src/lib.rs", "rust_line": 10, "go_test": "TestSomeBehaviour"}
            ]
        }
        report = RECONCILE.reconcile(inventory, anchors)
        self.assertEqual(1, len(report["unrecorded"]))
        self.assertEqual([], report["already_recorded"])

    def test_already_recorded_when_entry_names_the_anchor_file(self) -> None:
        inventory = self.inventory({
            "internal/api/routes_test.go:42:TestSomeBehaviour": {
                "status": "[x]",
                "rust_entry": "crates/x/src/lib.rs::some_case",
                "conclusion": "",
                "command": "",
                "evidence_type": "function_exact",
            }
        })
        anchors = {
            ("internal/api/routes_test.go", 42): [
                {"rust_file": "crates/x/src/lib.rs", "rust_line": 10, "go_test": "TestSomeBehaviour"}
            ]
        }
        report = RECONCILE.reconcile(inventory, anchors)
        self.assertEqual([], report["unrecorded"])
        self.assertEqual(1, len(report["already_recorded"]))
        self.assertEqual([], report["stale_anchor"])

    def test_stale_anchor_when_recorded_entry_points_elsewhere(self) -> None:
        inventory = self.inventory({
            "internal/api/routes_test.go:42:TestSomeBehaviour": {
                "status": "[x]",
                "rust_entry": "crates/moved/src/lib.rs::some_case",
                "conclusion": "",
                "command": "",
                "evidence_type": "function_exact",
            }
        })
        anchors = {
            ("internal/api/routes_test.go", 42): [
                {"rust_file": "crates/x/src/lib.rs", "rust_line": 10, "go_test": "TestSomeBehaviour"}
            ]
        }
        report = RECONCILE.reconcile(inventory, anchors)
        self.assertEqual(1, len(report["stale_anchor"]))
        self.assertEqual([], report["already_recorded"])

    def test_unknown_go_line_is_reported_separately(self) -> None:
        anchors = {
            ("internal/api/gone_test.go", 99): [
                {"rust_file": "crates/x/src/lib.rs", "rust_line": 3, "go_test": "TestGone"}
            ]
        }
        report = RECONCILE.reconcile({}, anchors)
        self.assertEqual(1, len(report["unknown_go_test"]))
        self.assertEqual([], report["unrecorded"])

    def test_name_mismatch_is_split_out_of_unrecorded(self) -> None:
        inventory = self.inventory({
            "internal/api/routes_test.go:42:TestSomeBehaviour": {
                "status": "[~]", "rust_entry": "", "conclusion": "",
                "command": "", "evidence_type": "missing",
            }
        })
        anchors = {
            ("internal/api/routes_test.go", 42): [
                {"rust_file": "crates/x/src/lib.rs", "rust_line": 10, "go_test": "TestDifferent"}
            ]
        }
        report = RECONCILE.reconcile(inventory, anchors)
        consistent, mismatched = RECONCILE.count_gradable(report)
        self.assertEqual([], consistent)
        self.assertEqual(1, len(mismatched))

    def test_unnamed_anchor_counts_as_consistent(self) -> None:
        inventory = self.inventory({
            "internal/api/routes_test.go:42:TestSomeBehaviour": {
                "status": "[~]", "rust_entry": "", "conclusion": "",
                "command": "", "evidence_type": "missing",
            }
        })
        anchors = {
            ("internal/api/routes_test.go", 42): [
                {"rust_file": "crates/x/src/lib.rs", "rust_line": 10, "go_test": None}
            ]
        }
        report = RECONCILE.reconcile(inventory, anchors)
        consistent, mismatched = RECONCILE.count_gradable(report)
        self.assertEqual(1, len(consistent))
        self.assertEqual([], mismatched)


class RenderTest(unittest.TestCase):
    """The markdown report states the read-only, human-review contract."""

    def test_report_is_marked_read_only_and_names_buckets(self) -> None:
        report = {
            "already_recorded": [],
            "unrecorded": [{
                "inventory_key": "internal/api/routes_test.go:42:TestSomeBehaviour",
                "evidence_type": "missing",
                "go_test": "TestSomeBehaviour",
                "anchors": [{"rust_file": "crates/x/src/lib.rs", "rust_line": 10,
                             "go_test": "TestSomeBehaviour"}],
            }],
            "unknown_go_test": [],
            "stale_anchor": [],
        }
        text = RECONCILE.render(report, 4451)
        self.assertIn("只读", text)
        self.assertIn("人工比对断言", text)
        self.assertIn("crates/x/src/lib.rs:10", text)


if __name__ == "__main__":
    unittest.main()
