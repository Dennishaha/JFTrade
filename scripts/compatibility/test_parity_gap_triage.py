from __future__ import annotations

import importlib.util
import os
import pathlib
import tempfile
import unittest


SCRIPT_PATH = pathlib.Path(__file__).with_name("parity_gap_triage.py")
SPEC = importlib.util.spec_from_file_location("parity_gap_triage", SCRIPT_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {SCRIPT_PATH}")
TRIAGE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(TRIAGE)


def missing_row(**overrides: object) -> dict:
    row = {"status": "[ ]", "rust_entry": "", "conclusion": "", "command": "",
           "evidence_type": "missing"}
    row.update(overrides)
    return row


class RustTestIndexTest(unittest.TestCase):
    """The index must see every shape of Rust test a mapping could point at."""

    def test_indexes_plain_and_parameterised_tokio_tests(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root / "crates" / "demo" / "src").mkdir(parents=True)
            (root / "crates" / "demo" / "src" / "lib.rs").write_text(
                "#[test]\n"
                "fn alpha_case() {}\n"
                "\n"
                "#[tokio::test]\n"
                "async fn beta_case() {}\n"
                "\n"
                '#[tokio::test(flavor = "multi_thread", worker_threads = 4)]\n'
                "async fn gamma_case() {}\n"
                "\n"
                "fn helper_not_a_test() {}\n",
                encoding="utf-8",
            )
            original = os.getcwd()
            os.chdir(root)
            try:
                index = TRIAGE.rust_test_index()
            finally:
                os.chdir(original)

        self.assertIn("alpha_case", index)
        self.assertIn("beta_case", index)
        # `#[tokio::test(...)]` with arguments is a real test and was silently
        # dropped before the regex accepted the argument list.
        self.assertIn("gamma_case", index)
        self.assertNotIn("helper_not_a_test", index)


class NameTokenTest(unittest.TestCase):
    def test_camel_case_and_acronyms_split_into_words(self) -> None:
        self.assertEqual(
            {"broker", "max", "quantity", "query", "snapshot", "trade"},
            TRIAGE.go_name_tokens("TestQueryBrokerMaxTradeQuantityReturnsSnapshot"),
        )
        self.assertEqual(
            {"k", "line", "data"},
            TRIAGE.go_name_tokens("TestKLineData"),
        )

    def test_stopwords_are_dropped(self) -> None:
        self.assertEqual(set(), TRIAGE.go_name_tokens("TestWorksWithTheCase"))


class CandidateSuggestionTest(unittest.TestCase):
    def test_threshold_is_inclusive_and_scores_are_reported(self) -> None:
        index = {"broker_read_projects_max_trade_quantity_snapshot": ["crates/a/src/lib.rs"]}
        candidates = TRIAGE.suggest_candidates(
            "TestQueryBrokerMaxTradeQuantityReturnsSnapshot", index
        )
        self.assertEqual(1, len(candidates))
        self.assertEqual("broker_read_projects_max_trade_quantity_snapshot",
                         candidates[0]["rust_test"])
        self.assertEqual(0.625, candidates[0]["similarity"])
        self.assertEqual(["crates/a/src/lib.rs"], candidates[0]["files"])

    def test_unrelated_names_are_not_suggested(self) -> None:
        # Exactly at the threshold (2 wanted tokens, 1 shared = 0.5) the
        # candidate is kept: the tool prefers recall and leaves precision to
        # the human reviewer.
        self.assertEqual(
            ["alpha_case"],
            [c["rust_test"] for c in
             TRIAGE.suggest_candidates("TestAlphaBeta", {"alpha_case": ["p"]})],
        )
        # One shared token out of three (0.333) is below the threshold.
        self.assertEqual(
            [],
            TRIAGE.suggest_candidates("TestAlphaBetaGamma", {"alpha_case": ["p"]}),
        )

    def test_candidates_are_capped_and_deterministic(self) -> None:
        index = {f"alpha_beta_variant_{c}": ["p"] for c in "cab"}
        names = [c["rust_test"] for c in TRIAGE.suggest_candidates("TestAlphaBeta", index, limit=2)]
        self.assertEqual(["alpha_beta_variant_a", "alpha_beta_variant_b"], names)


class TriageTest(unittest.TestCase):
    """Anchors outrank name guesses; every missing row lands in exactly one class."""

    def test_every_missing_row_lands_in_exactly_one_class(self) -> None:
        inventory = {
            "internal/a/x_test.go:1:TestAlphaBeta": missing_row(),
            "internal/a/x_test.go:2:TestGammaDelta": missing_row(),
            "internal/a/x_test.go:3:TestUnrelatedSubject": missing_row(),
        }
        anchors = {
            ("internal/a/x_test.go", 1): [
                {"rust_file": "crates/a/src/lib.rs", "rust_line": 5, "go_test": "TestAlphaBeta"}
            ]
        }
        index = {"alpha_beta_case": ["crates/a/src/lib.rs"],
                 "gamma_delta_case": ["crates/a/src/lib.rs"]}
        classes = TRIAGE.triage(inventory, index, anchors)

        keys = [row["inventory_key"] for rows in classes.values() for row in rows]
        self.assertEqual(len(inventory), len(keys))
        self.assertEqual(len(keys), len(set(keys)))
        self.assertEqual(1, len(classes["anchor_present"]))
        self.assertEqual(1, len(classes["candidate_test_found"]))
        self.assertEqual(1, len(classes["needs_test_or_boundary"]))

    def test_anchor_present_wins_over_a_matching_candidate(self) -> None:
        inventory = {"internal/a/x_test.go:1:TestAlphaBeta": missing_row()}
        anchors = {
            ("internal/a/x_test.go", 1): [
                {"rust_file": "crates/a/src/lib.rs", "rust_line": 5, "go_test": "TestAlphaBeta"}
            ]
        }
        index = {"alpha_beta_case": ["crates/a/src/lib.rs"]}
        classes = TRIAGE.triage(inventory, index, anchors)
        self.assertEqual([], classes["candidate_test_found"])
        self.assertEqual(1, len(classes["anchor_present"]))

    def test_recorded_rows_are_not_triaged(self) -> None:
        inventory = {
            "internal/a/x_test.go:1:TestAlphaBeta": missing_row(
                evidence_type="function_exact", rust_entry="crates/a/src/lib.rs::alpha_beta_case"
            ),
            "internal/a/x_test.go:2:TestGammaDelta": missing_row(evidence_type="boundary"),
            "internal/a/x_test.go:3:TestEpsilonZeta": missing_row(
                evidence_type="partial", rust_entry="crates/a/src/lib.rs::epsilon_zeta_case"
            ),
        }
        classes = TRIAGE.triage(inventory, {}, {})
        self.assertEqual(0, sum(len(rows) for rows in classes.values()))


class SelfCheckTest(unittest.TestCase):
    """The self-check turns recorded rows into a labelled sample."""

    def test_recall_counts_only_rows_naming_a_rust_test(self) -> None:
        inventory = {
            "a/x_test.go:1:TestAlphaBeta": {
                "evidence_type": "function_exact",
                "rust_entry": "crates/a/src/lib.rs::alpha_beta_case",
            },
            "a/x_test.go:2:TestGammaDelta": {
                "evidence_type": "partial",
                "rust_entry": "crates/a/src/lib.rs::totally_other_name",
            },
            "a/x_test.go:3:TestEpsilonZeta": {
                "evidence_type": "boundary",
                "rust_entry": "crates/a/src/lib.rs::epsilon_zeta_case",
            },
            "a/x_test.go:4:TestEtaTheta": {
                "evidence_type": "function_exact",
                "rust_entry": "",
            },
        }
        index = {"alpha_beta_case": ["crates/a/src/lib.rs"]}
        score = TRIAGE.self_check(inventory, index)

        # Boundary rows and rows without a rust_entry are not labelled samples.
        self.assertEqual(3, score["recorded_rows"])
        self.assertEqual(1, score["recovered"])
        self.assertEqual(0, score["not_recovered"])
        self.assertEqual(1, score["no_suggestion"])
        self.assertEqual(1.0, score["recall"])

    def test_recall_is_none_without_comparable_rows(self) -> None:
        score = TRIAGE.self_check({}, {})
        self.assertIsNone(score["recall"])


class RenderTest(unittest.TestCase):
    """The report must state the read-only, human-review contract."""

    def classes(self) -> dict:
        return {
            "anchor_present": [{
                "inventory_key": "internal/api/routes_test.go:42:TestSomeBehaviour",
                "go_file": "internal/api/routes_test.go", "go_line": 42,
                "go_test": "TestSomeBehaviour",
                "anchors": [{"rust_file": "crates/x/src/lib.rs", "rust_line": 10,
                             "go_test": "TestSomeBehaviour"}],
            }],
            "candidate_test_found": [{
                "inventory_key": "internal/api/orders_test.go:7:TestQueryBrokerMaxTradeQuantityReturnsSnapshot",
                "go_file": "internal/api/orders_test.go", "go_line": 7,
                "go_test": "TestQueryBrokerMaxTradeQuantityReturnsSnapshot",
                "candidates": [{"rust_test": "broker_read_projects_max_trade_quantity_snapshot",
                                "similarity": 0.625, "files": ["crates/a/src/lib.rs"]}],
            }],
            "needs_test_or_boundary": [{
                "inventory_key": "internal/api/unknown_test.go:9:TestNothingSimilar",
                "go_file": "internal/api/unknown_test.go", "go_line": 9,
                "go_test": "TestNothingSimilar",
            }],
        }

    def score(self) -> dict:
        return {"recorded_rows": 3, "recovered": 2, "not_recovered": 1,
                "no_suggestion": 0, "recall": 2 / 3}

    def test_report_is_read_only_and_forbids_promotion_from_candidates(self) -> None:
        text = TRIAGE.render(self.classes(), self.score(), 4451)
        self.assertIn("只读", text)
        self.assertIn("不修改 `manual-test-mappings.json`", text)
        self.assertIn("人工比对断言", text)
        self.assertIn("不得", text)

    def test_report_counts_every_class_and_shows_evidence(self) -> None:
        text = TRIAGE.render(self.classes(), self.score(), 4451)
        self.assertIn("清单总条目：4451", text)
        self.assertIn("`anchor_present`", text)
        self.assertIn("crates/x/src/lib.rs:10", text)
        self.assertIn("broker_read_projects_max_trade_quantity_snapshot", text)
        self.assertIn("TestNothingSimilar", text)
        self.assertIn("66.7%", text)


if __name__ == "__main__":
    unittest.main()
