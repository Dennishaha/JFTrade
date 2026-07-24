#!/usr/bin/env python3
"""Generate the server-side StockScreen factor catalog from Futu's SDK.

Usage:
  python3 scripts/generate-futu-stock-screen-catalog.py \
    /path/to/futu/quote/stock_screen_const.py \
    pkg/researchscreen/catalog_generated.go
"""

from __future__ import annotations

import ast
import hashlib
import pathlib
import re
import sys


FAMILIES = {
    "SimpleField": ("field", True, False, False, "enum"),
    "BasicProperty": ("basic", False, True, True, ""),
    "SimpleProperty": ("simple", True, True, True, "interval"),
    "CumulativeProperty": ("cumulative", True, True, True, "interval"),
    "FinancialProperty": ("financial", True, True, True, "interval"),
    "Indicator": ("indicator", True, True, True, "position"),
    "Pattern": ("pattern", True, False, False, "pattern"),
    "FeaturedProperty": ("featured", True, True, True, "interval_or_set"),
    "BrokerProperty": ("broker", True, True, True, "interval"),
    "OptionProperty": ("option", True, True, True, "interval"),
    "KlineShapeProperty": ("kline_shape", True, True, True, "set"),
}

AUXILIARY_ENUMS = {
    "ScrMarket": "market",
    "ScrSortDir": "sort_direction",
    "Period": "period",
    "Position": "position",
    "Term": "term",
    "RangePeriod": "range_period",
    "RecentDuration": "recent_duration",
    "FutureDuration": "future_duration",
    "CashFlowPeriod": "cash_flow_period",
    "OptionHVPeriod": "option_hv_period",
    "KlineShapeType": "kline_shape_type",
}

UNIT_OVERRIDES = {
    "simple.long_margin_allowed": "",
    "simple.short_margin_allowed": "",
    "simple.price_to_52w_high": "percent",
    "simple.price_to_52w_low": "percent",
    "simple.high_to_52w_high": "percent",
    "simple.low_to_52w_low": "percent",
    "simple.volume_ratio": "",
    "simple.pe_annual": "",
    "simple.pe_ttm": "",
    "simple.pb": "",
    "financial.equity_multiplier": "",
    "financial.money_turnover_cycle": "days",
    "financial.stockholder_profit_cagr": "percent",
    "financial.operating_profit_cagr": "percent",
    "financial.free_cash_cagr": "percent",
    "financial.total_assets_cagr": "percent",
    "financial.operating_revenue_cash_cover": "percent",
    "financial.surprise_revenue_date": "timestamp",
    "financial.surprise_revenue_term": "",
    "financial.surprise_revenue_post_period": "",
    "financial.surprise_revenue_date_v2": "timestamp",
    "financial.surprise_revenue_post_period_v2": "",
    "featured.cash_flow_net_in_count": "count",
    "featured.cash_flow_net_out_count": "count",
    "featured.cash_flow_main_in_count": "count",
    "featured.cash_flow_main_out_count": "count",
}

EXPLICIT_QUOTE_PRICE_FACTORS = {
    "simple.last_close",
    "simple.high",
    "simple.low",
    "simple.last_close_hp",
    "simple.high_hp",
    "simple.low_hp",
    "featured.morningstar_fair_value",
    "kline_shape.support_level",
    "kline_shape.pressure_level",
}

PRICE_DISPLAY_FACTORS = EXPLICIT_QUOTE_PRICE_FACTORS | {
    "simple.price",
    "simple.open_price",
    "simple.bid_price",
    "simple.ask_price",
    "simple.lot_price",
    "simple.price_hp",
    "simple.open_price_hp",
    "simple.bid_price_hp",
    "simple.ask_price_hp",
    "simple.lot_price_hp",
    "simple.before_price",
    "simple.before_price_change",
    "simple.after_price",
    "simple.after_price_change",
    "simple.before_price_hp",
    "simple.before_price_change_hp",
    "simple.after_price_hp",
    "simple.after_price_change_hp",
    "simple.overnight_price",
    "simple.overnight_price_change",
    "cumulative.price_change",
    "cumulative.amplitude",
    "cumulative.price_change_hp",
    "indicator.price",
    "featured.analyst_target_price",
}


def label_for(line: str, name: str) -> str:
    comment = line.partition("#")[2].strip()
    comment = re.sub(r"[（(]倍率[:：].*?[）)]", "", comment).strip()
    return comment or name.replace("_", " ").title()


def value_type(name: str, label: str) -> str:
    text = f"{name} {label}".upper()
    if any(word in text for word in ("DATE", "TIME", "DAYS", "COUNT", "NUM", "VOLUME", "SHARES")):
        return "integer"
    if any(word in text for word in ("HAS_", "ALLOWED", "IS_", "TYPE", "STATUS", "SIGNAL", "SHAPE")):
        return "enum"
    return "number"


def is_quote_price_factor(key: str) -> bool:
    return (
        key in EXPLICIT_QUOTE_PRICE_FACTORS
        or key.startswith("indicator.ma")
        or key.startswith("indicator.ema")
        or key.startswith("indicator.boll")
    )


def unit_for(family: str, name: str, label: str) -> str:
    key = f"{family}.{name.lower()}"
    if key in UNIT_OVERRIDES:
        return UNIT_OVERRIDES[key]
    if is_quote_price_factor(key):
        return "currency"
    text = f"{name} {label}".upper()
    if any(word in text for word in ("DATE", "TIME", "日期", "时间")):
        return "timestamp"
    if any(word in text for word in ("VOLUME", "SHARES", "成交量", "股数")):
        return "shares"
    if any(word in text for word in ("RATE", "RATIO", "PCT", "YIELD", "MARGIN", "ROE", "ROA", "ROIC", "比例", "率", "涨跌幅")):
        return "percent"
    if any(word in text for word in ("PRICE", "CAP", "TURNOVER", "PROFIT", "REVENUE", "CASH", "DEBT", "ASSET", "EQUITY", "价格", "市值", "金额", "利润", "收入")):
        return "currency"
    return ""


def currency_basis(key: str, family: str, unit: str) -> str:
    if unit != "currency":
        return ""
    if family == "financial" and key != "financial.float_market_cap":
        return "reporting"
    return "quote"


def display_format(key: str, value_kind: str, unit: str) -> str:
    if unit == "currency":
        return "price" if key in PRICE_DISPLAY_FACTORS or is_quote_price_factor(key) else "compact_amount"
    if unit == "percent":
        return "percent"
    if unit == "timestamp":
        return "timestamp"
    if unit in ("shares", "count", "days") or value_kind == "integer":
        return "integer"
    return "number" if value_kind == "number" else ""


def parameters(family: str) -> list[tuple[str, str, str]]:
    return {
        "cumulative": [("days", "integer", ""), ("periodAverage", "integer", "")],
        "financial": [
            ("term", "integer", "term"), ("duration", "integer", ""), ("year", "integer", ""),
            ("periodAverage", "integer", ""), ("futureDuration", "integer", "future_duration"),
        ],
        "indicator": [("period", "integer", "period"), ("indicatorParams", "integer_array", "")],
        "featured": [("period", "integer", "period"), ("rangePeriod", "integer", "range_period"), ("firstCustomParam", "integer", "")],
        "broker": [("days", "integer", ""), ("brokerParam", "string", "")],
        "option": [("optionParam", "union", ""), ("optionHvPeriod", "integer", "option_hv_period")],
        "kline_shape": [("period", "integer", "period")],
    }.get(family, [])


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("expected SOURCE and OUTPUT")
    source = pathlib.Path(sys.argv[1])
    output = pathlib.Path(sys.argv[2])
    text = source.read_text(encoding="utf-8")
    lines = text.splitlines()
    tree = ast.parse(text)
    entries: list[tuple] = []
    enums: dict[str, list[tuple[str, int, str]]] = {}
    for node in tree.body:
        if isinstance(node, ast.ClassDef) and node.name in AUXILIARY_ENUMS:
            enum_name = AUXILIARY_ENUMS[node.name]
            enum_values = []
            for item in node.body:
                if not isinstance(item, ast.Assign) or len(item.targets) != 1:
                    continue
                target = item.targets[0]
                if not isinstance(target, ast.Name) or not isinstance(item.value, ast.Constant):
                    continue
                if not isinstance(item.value.value, int):
                    continue
                enum_values.append((
                    target.id.lower(), item.value.value,
                    label_for(lines[item.lineno - 1], target.id),
                ))
            enums[enum_name] = enum_values
        if not isinstance(node, ast.ClassDef) or node.name not in FAMILIES:
            continue
        family, can_filter, can_retrieve, can_sort, filter_kind = FAMILIES[node.name]
        for item in node.body:
            if not isinstance(item, ast.Assign) or len(item.targets) != 1:
                continue
            target = item.targets[0]
            if not isinstance(target, ast.Name) or not isinstance(item.value, ast.Constant):
                continue
            if not isinstance(item.value.value, int) or item.value.value <= 0:
                continue
            name = target.id
            label = label_for(lines[item.lineno - 1], name)
            key = f"{family}.{name.lower()}"
            kind = value_type(name, label)
            unit = unit_for(family, name, label)
            entries.append((
                key, family, label, kind, unit, currency_basis(key, family, unit),
                display_format(key, kind, unit), filter_kind, can_filter,
                can_retrieve, can_sort, item.value.value, parameters(family),
            ))
    digest = hashlib.sha256(text.encode()).hexdigest()
    rows = []
    for key, family, label, kind, unit, basis, display, filter_kind, filt, retrieve, sort, provider_id, params in entries:
        param_go = ""
        if params:
            param_go = ", Parameters: []ParameterDescriptor{" + ", ".join(
                f'{{Name: "{name}", Type: "{typ}", Enum: "{enum}"}}' for name, typ, enum in params
            ) + "}"
        semantics_go = ""
        if basis:
            semantics_go += f', CurrencyBasis: "{basis}"'
        if display:
            semantics_go += f', DisplayFormat: "{display}"'
        rows.append(
            f'\t{{Key: "{key}", Category: "{family}", Label: {label!r}, '
            f'ValueType: "{kind}", Unit: "{unit}"{semantics_go}, FilterKind: "{filter_kind}", '
            f"Filter: {str(filt).lower()}, Retrieve: {str(retrieve).lower()}, "
            f"Sort: {str(sort).lower()}, ProviderID: {provider_id}{param_go}}},"
        )
    enum_rows = []
    for enum_name, values in sorted(enums.items()):
        options = ", ".join(
            f'{{Key: "{key}", Value: {value}, Label: {label!r}}}'
            for key, value, label in values
        )
        enum_rows.append(f'\t"{enum_name}": {{{options}}},')
    content = "\n".join([
        "// Code generated by scripts/generate-futu-stock-screen-catalog.py; DO NOT EDIT.",
        f"// Source: futu-api 10.9.6908 stock_screen_const.py sha256:{digest}",
        "",
        "package researchscreen",
        "",
        "var generatedFactors = []FactorDescriptor{",
        *rows,
        "}",
        "",
        "var generatedEnums = map[string][]EnumOption{",
        *enum_rows,
        "}",
        "",
    ]).replace("'", '"')
    output.write_text(content, encoding="utf-8")
    print(f"generated {len(entries)} factors in {output}")


if __name__ == "__main__":
    main()
