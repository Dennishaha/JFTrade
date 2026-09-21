//! Reference-parity contracts for the fixed-point decimal used by orders,
//! balances and market snapshots.

use jftrade_kernel::Fixed8;

/// The wire form keeps eight fractional digits (never trimmed) so a stored
/// zero, a rounded value and a legacy numeric payload all round-trip to the
/// same text the reference implementation emits.
// Parity: go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:231 TestJson
#[test]
fn json_keeps_eight_digit_text_and_accepts_legacy_payloads() {
    let encode = |input: &str| {
        serde_json::to_string(&input.parse::<Fixed8>().expect("parse fixed8")).expect("encode")
    };
    assert_eq!(encode("0"), "0.00000000");
    assert_eq!(encode("1.00000003"), "1.00000003");
    assert_eq!(encode("1.000000003"), "1.00000000");
    assert_eq!(encode("1.000000008"), "1.00000000");
    assert_eq!(encode("0.999999999"), "0.99999999");
    assert_eq!(encode("1.2e-9"), "0.00000000");

    let legacy: Fixed8 = serde_json::from_str("0.00153917575").expect("legacy numeric");
    assert_eq!(legacy.fixed_text(), "0.00153917");
    assert_eq!(
        legacy,
        "0.00153917575"
            .parse::<Fixed8>()
            .expect("parse legacy text")
    );
    let six_e_8: Fixed8 = serde_json::from_str("6e-8").expect("6e-8");
    let sixty_two: Fixed8 = serde_json::from_str("0.000062").expect("0.000062");
    assert_eq!(
        sixty_two
            .checked_sub(six_e_8)
            .expect("difference")
            .storage_text(),
        "0.00006194"
    );

    let infinity: Fixed8 = serde_json::from_str("\"inf\"").expect("inf");
    let plus_infinity: Fixed8 = serde_json::from_str("\"+Inf\"").expect("+Inf");
    assert_eq!(infinity, Fixed8::POS_INFINITY);
    assert_eq!(infinity, plus_infinity);
}

/// Percent suffixes, scientific notation, empty input and the four
/// non-finite spellings all resolve to the documented canonical values.
// Parity: go:452dea11:pkg/bbgo/fixedpoint/dec_test.go:206 TestFromString
#[test]
fn parsing_normalizes_percent_scientific_empty_and_non_finite_forms() {
    for (input, expected) in [
        ("0.004075", "0.004075"),
        ("0.03", "0.03"),
        ("0.75%", "0.0075"),
        ("1.1e-7", "0.00000011"),
        (".0%", "0"),
        ("", "0"),
    ] {
        assert_eq!(
            input
                .parse::<Fixed8>()
                .expect("parse fixed8")
                .storage_text(),
            expected,
            "{input}"
        );
    }
    for spelling in ["inf", "Inf", "INF", "iNF"] {
        assert_eq!(
            spelling.parse::<Fixed8>().expect("parse inf"),
            Fixed8::POS_INFINITY,
            "{spelling}"
        );
        assert_eq!(
            format!("+{spelling}")
                .parse::<Fixed8>()
                .expect("parse +inf"),
            Fixed8::POS_INFINITY,
            "+{spelling}"
        );
        assert_eq!(
            format!("-{spelling}")
                .parse::<Fixed8>()
                .expect("parse -inf"),
            Fixed8::NEG_INFINITY,
            "-{spelling}"
        );
    }
}
