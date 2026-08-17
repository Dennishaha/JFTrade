"""Curated macro indicator catalog.

Each entry pins a stable ``indicator_id`` to one akshare 1.18.91 function and
its date/value/predict/previous columns.  Column names below were verified
against the installed package sources:

- 金十 base frames (``__macro_china_base_func`` / ``__macro_usa_base_func``)
  carry 商品/日期/今值/预测值/前值.
- ``macro_usa_cpi_yoy`` (Eastmoney) carries 时间/发布日期/现值/前值.
- ``macro_china_lpr`` carries TRADE_DATE/LPR1Y/LPR5Y.
- ``macro_china_new_financial_credit`` carries 月份/当月/当月-同比增长.

unit_type: 1 = percentage, 3 = index/absolute value.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class MacroIndicatorSpec:
    indicator_id: str
    name: str
    region: str
    unit: str
    unit_type: int
    frequency: str
    category: str
    function: str  # akshare function name, e.g. "macro_china_cpi_yearly"
    date_column: str
    value_column: str
    predict_column: str | None = None
    previous_column: str | None = None


def _jin10(
    indicator_id: str,
    name: str,
    region: str,
    unit: str,
    unit_type: int,
    frequency: str,
    category: str,
    function: str,
) -> MacroIndicatorSpec:
    """金十数据中心指标：日期/今值/预测值/前值 四列。"""
    return MacroIndicatorSpec(
        indicator_id=indicator_id,
        name=name,
        region=region,
        unit=unit,
        unit_type=unit_type,
        frequency=frequency,
        category=category,
        function=function,
        date_column="日期",
        value_column="今值",
        predict_column="预测值",
        previous_column="前值",
    )


INDICATORS: tuple[MacroIndicatorSpec, ...] = (
    # ---- 中国 ----
    # 中国CPI年率报告（金十 datacenter attr_id=56）
    _jin10("cn_cpi_yoy", "CPI同比", "中国", "%", 1, "monthly", "中国·物价", "macro_china_cpi_yearly"),
    # 中国PPI年率报告（金十 attr_id=60）
    _jin10("cn_ppi_yoy", "PPI同比", "中国", "%", 1, "monthly", "中国·物价", "macro_china_ppi_yearly"),
    # 中国官方制造业PMI（金十 attr_id=65）
    _jin10("cn_pmi", "官方制造业PMI", "中国", "点", 3, "monthly", "中国·景气", "macro_china_pmi_yearly"),
    # 中国官方非制造业PMI（金十 attr_id=75）
    _jin10("cn_non_man_pmi", "官方非制造业PMI", "中国", "点", 3, "monthly", "中国·景气", "macro_china_non_man_pmi"),
    # 中国GDP年率报告（金十 attr_id=57，季度发布）
    _jin10("cn_gdp_yoy", "GDP同比", "中国", "%", 1, "quarterly", "中国·经济总量", "macro_china_gdp_yearly"),
    # 中国以美元计算出口年率报告（金十 attr_id=66）
    _jin10("cn_exports_yoy", "出口同比(美元)", "中国", "%", 1, "monthly", "中国·经济总量", "macro_china_exports_yoy"),
    # 中国M2货币供应年率报告（金十 attr_id=59）
    _jin10("cn_m2_yoy", "M2货币供应同比", "中国", "%", 1, "monthly", "中国·货币信贷", "macro_china_m2_yearly"),
    # 中国新增信贷数据（东财 RPT_ECONOMY_RMB_LOAN，当月新增人民币贷款）
    MacroIndicatorSpec(
        indicator_id="cn_new_credit",
        name="新增人民币贷款",
        region="中国",
        unit="亿元",
        unit_type=3,
        frequency="monthly",
        category="中国·货币信贷",
        function="macro_china_new_financial_credit",
        date_column="月份",
        value_column="当月",
    ),
    # LPR 一年期（东财 RPTA_WEB_RATE，LPR1Y 列）
    MacroIndicatorSpec(
        indicator_id="cn_lpr_1y",
        name="LPR一年期",
        region="中国",
        unit="%",
        unit_type=1,
        frequency="monthly",
        category="中国·货币信贷",
        function="macro_china_lpr",
        date_column="TRADE_DATE",
        value_column="LPR1Y",
    ),
    # ---- 美国 ----
    # 美国CPI年率（东财 RPT_ECONOMICVALUE_USA，EMG00000733）
    MacroIndicatorSpec(
        indicator_id="us_cpi_yoy",
        name="CPI同比",
        region="美国",
        unit="%",
        unit_type=1,
        frequency="monthly",
        category="美国·物价",
        function="macro_usa_cpi_yoy",
        date_column="时间",
        value_column="现值",
        previous_column="前值",
    ),
    # 美国生产者物价指数PPI报告（金十 attr_id=37）
    _jin10("us_ppi_yoy", "PPI同比", "美国", "%", 1, "monthly", "美国·物价", "macro_usa_ppi"),
    # 美国非农就业人数报告（金十 attr_id=33）
    _jin10("us_non_farm", "非农就业人数", "美国", "千人", 3, "monthly", "美国·就业", "macro_usa_non_farm"),
    # 美国失业率报告（金十 attr_id=47）
    _jin10("us_unemployment", "失业率", "美国", "%", 1, "monthly", "美国·就业", "macro_usa_unemployment_rate"),
    # 美国零售销售月率报告（金十 attr_id=39）
    _jin10("us_retail_sales", "零售销售月率", "美国", "%", 1, "monthly", "美国·消费与景气", "macro_usa_retail_sales"),
    # 美国ISM制造业PMI报告（金十 attr_id=28）
    _jin10("us_ism_pmi", "ISM制造业PMI", "美国", "点", 3, "monthly", "美国·消费与景气", "macro_usa_ism_pmi"),
    # 美国国内生产总值(GDP)报告（金十 attr_id=53）
    _jin10("us_gdp", "GDP(环比折年率)", "美国", "%", 1, "quarterly", "美国·消费与景气", "macro_usa_gdp_monthly"),
)

_BY_ID = {spec.indicator_id: spec for spec in INDICATORS}
_CATEGORY_ORDER = (
    "中国·物价",
    "中国·景气",
    "中国·经济总量",
    "中国·货币信贷",
    "美国·物价",
    "美国·就业",
    "美国·消费与景气",
)


def indicator_by_id(indicator_id: str) -> MacroIndicatorSpec | None:
    return _BY_ID.get(indicator_id)


def category_order() -> tuple[str, ...]:
    return _CATEGORY_ORDER
