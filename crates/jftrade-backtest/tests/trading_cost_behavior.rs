use serde_json::{Value, json};

use jftrade_backtest::run_json;
use jftrade_kernel::Fixed8;

fn candle(index: usize, open: &str, volume: &str) -> Value {
    let minute = 30 + index;
    json!({
        "start": format!("2026-06-30T13:{minute:02}:00Z"),
        "end": format!("2026-06-30T13:{minute:02}:59.999Z"),
        "open": open,
        "high": open,
        "low": open,
        "close": open,
        "volume": volume,
    })
}

fn run_case(mut case: Value) -> Result<Value, String> {
    let corpus = json!({
        "version": 1,
        "cases": [case.take()],
    });
    let input = serde_json::to_vec(&corpus).map_err(|error| error.to_string())?;
    let output = run_json(&input).map_err(|error| error.to_string())?;
    let output: Value = serde_json::from_slice(&output).map_err(|error| error.to_string())?;
    output
        .get("cases")
        .and_then(Value::as_array)
        .and_then(|cases| cases.first())
        .cloned()
        .ok_or_else(|| "backtest output did not contain a case".to_owned())
}

fn base_case(id: &str, quote_currency: &str, fee_rules: Vec<Value>, quantity: &str) -> Value {
    json!({
        "id": id,
        "symbol": "US.AAPL",
        "baseCurrency": "AAPL",
        "quoteCurrency": quote_currency,
        "initialBalance": "100000",
        "market": {
            "tickSize": "0.01",
            "quantityStep": "1",
            "minQuantity": "1"
        },
        "feeRules": fee_rules,
        "candles": [
            candle(0, "100", "1000"),
            candle(1, "100", "1000")
        ],
        "intents": [{
            "barIndex": 0,
            "action": "submit",
            "id": "entry",
            "side": "buy",
            "orderType": "market",
            "quantity": quantity
        }]
    })
}

fn fee_breakdown<'a>(case: &'a Value, group: &str, rule_id: &str) -> &'a Value {
    case["feeBreakdown"]
        .as_array()
        .expect("fee breakdown array")
        .iter()
        .find(|entry| entry["group"] == group && entry["ruleId"] == rule_id)
        .unwrap_or_else(|| panic!("missing {group}/{rule_id} in {}", case["feeBreakdown"]))
}

fn assert_fixed8_near(value: &Value, expected: &str) {
    let actual = value
        .as_str()
        .unwrap_or_else(|| panic!("expected fixed-point string, got {value}"))
        .parse::<Fixed8>()
        .unwrap_or_else(|error| panic!("invalid fixed-point output {value}: {error}"));
    let expected = expected
        .parse::<Fixed8>()
        .unwrap_or_else(|error| panic!("invalid fixed-point expectation {expected}: {error}"));
    let difference = (actual.scaled() - expected.scaled()).abs();
    assert!(
        difference <= 2,
        "fixed-point output {actual} differs from {expected} by {difference} scaled units"
    );
}

#[test]
fn hong_kong_market_rules_round_currency_unit_and_keep_fee_groups_separate() {
    let rules = vec![
        json!({
            "id": "futu_hk_hk_commission",
            "label": "Futu HK commission",
            "group": "broker",
            "side": "both",
            "basis": "notional",
            "rate": "0.0003",
            "minAmount": "3"
        }),
        json!({
            "id": "futu_hk_hk_platform_fee",
            "label": "Futu HK platform fee",
            "group": "broker",
            "side": "both",
            "basis": "order",
            "fixedAmount": "15"
        }),
        json!({
            "id": "hkex_hk_settlement_fee",
            "label": "HKEX settlement fee",
            "group": "market",
            "side": "both",
            "basis": "notional",
            "rate": "0.000042"
        }),
        json!({
            "id": "hkex_hk_trading_fee",
            "label": "HKEX trading fee",
            "group": "market",
            "side": "both",
            "basis": "notional",
            "rate": "0.0000565"
        }),
        json!({
            "id": "sfc_hk_transaction_levy",
            "label": "SFC transaction levy",
            "group": "market",
            "side": "both",
            "basis": "notional",
            "rate": "0.000027"
        }),
        json!({
            "id": "afrc_hk_transaction_levy",
            "label": "AFRC transaction levy",
            "group": "market",
            "side": "both",
            "basis": "notional",
            "rate": "0.0000015"
        }),
        json!({
            "id": "hk_stamp_duty",
            "label": "Hong Kong stamp duty",
            "group": "market",
            "side": "both",
            "basis": "notional",
            "rate": "0.001",
            "rounding": "ceil_currency_unit"
        }),
    ];
    let mut case = base_case("hong-kong-rounding", "HKD", rules, "100");
    case["symbol"] = json!("HK.00700");
    let result = run_case(case).expect("run Hong Kong fee case");

    assert_eq!(result["totalBrokerFees"], "18");
    assert_fixed8_near(&result["totalMarketFees"], "11.27");
    assert_fixed8_near(&result["totalFees"], "29.27");
    assert_eq!(
        fee_breakdown(&result, "broker", "futu_hk_hk_commission")["amount"],
        "3"
    );
    assert_eq!(
        fee_breakdown(&result, "broker", "futu_hk_hk_platform_fee")["amount"],
        "15"
    );
    assert_eq!(
        fee_breakdown(&result, "market", "hk_stamp_duty")["amount"],
        "10"
    );
}

#[test]
fn fee_rule_text_is_case_insensitive_and_trimmed_before_calculation() {
    let rule = json!({
        "id": "trimmed-rule",
        "label": "Trimmed rule",
        "group": "broker",
        "side": " SELL ",
        "basis": " NOTIONAL ",
        "rate": "0.0001001",
        "rounding": " CEIL_CENT "
    });
    let mut case = base_case("normalized-rule-fields", "USD", vec![rule], "100");
    case["intents"][0]["side"] = json!("sell");
    let result = run_case(case).expect("run normalized fee case");

    assert_eq!(result["totalBrokerFees"], "1.01");
    assert_eq!(result["totalMarketFees"], "0");
    assert_eq!(
        fee_breakdown(&result, "broker", "trimmed-rule")["amount"],
        "1.01"
    );
}

#[test]
fn us_market_rules_apply_broker_rate_caps_and_sell_side_regulatory_fees() {
    let rules = vec![
        json!({
            "id": "futu_hk_us_commission",
            "label": "Futu HK US commission",
            "group": "broker",
            "side": "both",
            "basis": "share",
            "fixedAmount": "0.0049",
            "minAmount": "0.99",
            "maxRate": "0.005"
        }),
        json!({
            "id": "futu_hk_us_platform_fee",
            "label": "Futu HK US platform fee",
            "group": "broker",
            "side": "both",
            "basis": "share",
            "fixedAmount": "0.005",
            "minAmount": "1",
            "maxRate": "0.005"
        }),
        json!({
            "id": "us_clearing_fee",
            "label": "US clearing fee",
            "group": "market",
            "side": "both",
            "basis": "share",
            "fixedAmount": "0.003"
        }),
        json!({
            "id": "sec_section_31_fee",
            "label": "SEC Section 31 fee",
            "group": "market",
            "side": "sell",
            "basis": "notional",
            "rate": "0.0000206"
        }),
        json!({
            "id": "finra_taf",
            "label": "FINRA TAF",
            "group": "market",
            "side": "sell",
            "basis": "share",
            "fixedAmount": "0.000195",
            "minAmount": "0.01",
            "maxAmount": "9.79"
        }),
        json!({
            "id": "cat_fee",
            "label": "CAT fee",
            "group": "market",
            "side": "both",
            "basis": "share",
            "fixedAmount": "0.000003"
        }),
    ];
    let mut case = base_case("us-cap-and-regulatory-fees", "USD", rules, "1000");
    case["candles"][0] = candle(0, "0.10", "10000");
    case["candles"][1] = candle(1, "0.10", "10000");
    case["intents"][0]["side"] = json!("sell");
    let result = run_case(case).expect("run US fee case");

    assert_eq!(result["totalBrokerFees"], "1");
    assert_fixed8_near(&result["totalMarketFees"], "3.20006");
    assert_fixed8_near(&result["totalFees"], "4.20006");
    assert_eq!(
        fee_breakdown(&result, "broker", "futu_hk_us_commission")["amount"],
        "0.5"
    );
    assert_eq!(
        fee_breakdown(&result, "broker", "futu_hk_us_platform_fee")["amount"],
        "0.5"
    );
    assert_fixed8_near(
        &fee_breakdown(&result, "market", "sec_section_31_fee")["amount"],
        "0.00206",
    );
    assert_eq!(
        fee_breakdown(&result, "market", "finra_taf")["amount"],
        "0.195"
    );
}

#[test]
fn per_order_minimum_is_charged_incrementally_across_partial_fills() {
    let rule = json!({
        "id": "broker-minimum",
        "label": "Broker minimum",
        "group": "broker",
        "side": "both",
        "basis": "notional",
        "rate": "0.001",
        "minAmount": "10"
    });
    let mut case = base_case("incremental-minimum", "USD", vec![rule], "1200");
    case["candles"] = json!([
        candle(0, "10", "1"),
        candle(1, "10", "1000"),
        candle(2, "10", "1000"),
        candle(3, "10", "10000")
    ]);
    let result = run_case(case).expect("run incremental minimum case");

    assert_eq!(result["totalBrokerFees"], "12");
    assert_eq!(result["totalMarketFees"], "0");
    assert_eq!(result["totalFees"], "12");
    let breakdown = fee_breakdown(&result, "broker", "broker-minimum");
    assert_eq!(breakdown["amount"], "12");
    assert_eq!(breakdown["count"], 2);
}

#[test]
fn unsupported_fee_basis_rounding_and_group_fail_closed_when_a_fill_uses_them() {
    let invalid_basis = base_case(
        "invalid-basis",
        "USD",
        vec![json!({
            "id": "invalid-basis",
            "label": "Invalid basis",
            "group": "broker",
            "basis": "contract_value",
            "rate": "0.001"
        })],
        "100",
    );
    let basis_error = run_case(invalid_basis).expect_err("unsupported basis must fail");
    assert!(basis_error.contains("unsupported basis"), "{basis_error}");

    let invalid_rounding = base_case(
        "invalid-rounding",
        "USD",
        vec![json!({
            "id": "invalid-rounding",
            "label": "Invalid rounding",
            "group": "broker",
            "basis": "notional",
            "rate": "0.001",
            "rounding": "bankers"
        })],
        "100",
    );
    let rounding_error = run_case(invalid_rounding).expect_err("unsupported rounding must fail");
    assert!(
        rounding_error.contains("unsupported fee rounding"),
        "{rounding_error}"
    );

    let invalid_group = base_case(
        "invalid-group",
        "USD",
        vec![json!({
            "id": "invalid-group",
            "label": "Invalid group",
            "group": "venue",
            "basis": "notional",
            "rate": "0.001"
        })],
        "100",
    );
    let group_error = run_case(invalid_group).expect_err("unsupported group must fail");
    assert!(group_error.contains("unsupported group"), "{group_error}");
}
