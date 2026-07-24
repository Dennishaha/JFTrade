from __future__ import annotations

import importlib.util
import pathlib
import unittest


SCRIPT_PATH = pathlib.Path(__file__).with_name(
    "generate-futu-stock-screen-catalog.py"
)
SPEC = importlib.util.spec_from_file_location("stock_screen_catalog_generator", SCRIPT_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {SCRIPT_PATH}")
GENERATOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(GENERATOR)


class StockScreenCatalogGeneratorTest(unittest.TestCase):
    def test_currency_semantics_distinguish_quote_and_reporting_values(self) -> None:
        price_unit = GENERATOR.unit_for("simple", "PRICE", "最新价格")
        self.assertEqual(price_unit, "currency")
        self.assertEqual(
            GENERATOR.currency_basis("simple.price", "simple", price_unit),
            "quote",
        )
        self.assertEqual(
            GENERATOR.display_format("simple.price", "number", price_unit),
            "price",
        )

        profit_unit = GENERATOR.unit_for("financial", "NET_PROFIT", "净利润")
        self.assertEqual(profit_unit, "currency")
        self.assertEqual(
            GENERATOR.currency_basis(
                "financial.net_profit",
                "financial",
                profit_unit,
            ),
            "reporting",
        )
        self.assertEqual(
            GENERATOR.display_format(
                "financial.net_profit",
                "number",
                profit_unit,
            ),
            "compact_amount",
        )

    def test_known_false_currency_units_are_corrected(self) -> None:
        cases = [
            ("financial", "EQUITY_MULTIPLIER", "权益乘数", ""),
            ("financial", "MONEY_TURNOVER_CYCLE", "资金周转周期(天)", "days"),
            (
                "financial",
                "STOCKHOLDER_PROFIT_CAGR",
                "归属普通股利润CAGR",
                "percent",
            ),
            (
                "financial",
                "SURPRISE_REVENUE_DATE",
                "REVENUE发布日 (时间戳秒)",
                "timestamp",
            ),
            (
                "featured",
                "CASH_FLOW_NET_IN_COUNT",
                "整体净流入次数",
                "count",
            ),
        ]
        for family, name, label, expected in cases:
            with self.subTest(name=name):
                self.assertEqual(
                    GENERATOR.unit_for(family, name, label),
                    expected,
                )

    def test_quote_price_overrides_cover_non_price_names(self) -> None:
        for family, name, label in [
            ("simple", "LAST_CLOSE", "昨收价"),
            ("indicator", "MA", "动态简单均线"),
            ("kline_shape", "SUPPORT_LEVEL", "支撑位"),
        ]:
            with self.subTest(name=name):
                unit = GENERATOR.unit_for(family, name, label)
                key = f"{family}.{name.lower()}"
                self.assertEqual(unit, "currency")
                self.assertEqual(
                    GENERATOR.display_format(key, "number", unit),
                    "price",
                )


if __name__ == "__main__":
    unittest.main()
