import importlib.util
import pathlib
import unittest


SCRIPT = pathlib.Path(__file__).with_name("run_parity_receipt.py")
SPEC = importlib.util.spec_from_file_location("run_parity_receipt", SCRIPT)
RUNNER = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(RUNNER)


class ParityReceiptCommandTest(unittest.TestCase):
    def test_builds_a_focused_nextest_command_for_multiple_packages_and_tests(self) -> None:
        command = RUNNER.build_command(
            ["jftrade-engine", "jftrade-calendar", "jftrade-engine"],
            ["alpha_case", "beta_case", "alpha_case"],
        )
        self.assertEqual(command[:5], [
            "env",
            "NEXTEST_EXPERIMENTAL_LIBTEST_JSON=1",
            "node",
            "scripts/quality/cargo-nextest.mjs",
            "run",
        ])
        self.assertEqual(command[5:9], ["-p", "jftrade-engine", "-p", "jftrade-calendar"])
        self.assertIn("--all-targets", command)
        self.assertIn("--locked", command)
        self.assertIn("--no-fail-fast", command)
        self.assertEqual(command[-1], "test(alpha_case) or test(beta_case)")

    def test_rejects_missing_scope_and_workspace_like_values(self) -> None:
        with self.assertRaisesRegex(ValueError, "at least one --package"):
            RUNNER.build_command([], ["case"])
        with self.assertRaisesRegex(ValueError, "at least one --test"):
            RUNNER.build_command(["jftrade-engine"], [])
        with self.assertRaisesRegex(ValueError, "invalid package name"):
            RUNNER.build_command(["--workspace"], ["case"])

    def test_rejects_test_filter_injection(self) -> None:
        with self.assertRaisesRegex(ValueError, "invalid test name"):
            RUNNER.build_command(["jftrade-engine"], ["case or test(other)"])

    def test_accepts_pnpm_argument_separator(self) -> None:
        args = RUNNER.parse_args(["--", "--package", "jftrade-engine", "--test", "case", "--dry-run"])
        self.assertEqual(args.package, ["jftrade-engine"])
        self.assertEqual(args.test, ["case"])
        self.assertTrue(args.dry_run)

    def test_passed_test_names_read_structured_nextest_events(self) -> None:
        path = pathlib.Path(self.id().replace("/", "_") + ".jsonl")
        try:
            path.write_text(
                '{"type":"test","event":"ok","name":"jftrade-engine::suite::case"}\n'
                '{"type":"test","event":"ok","name":"jftrade-store-sqlite::aggregation$aggregated_case"}\n'
                '{"type":"test","event":"failed","name":"jftrade-engine::suite::broken"}\n',
                encoding="utf-8",
            )
            self.assertEqual(RUNNER.passed_test_names(path), {"case", "aggregated_case"})
        finally:
            path.unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
