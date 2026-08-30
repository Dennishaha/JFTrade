use super::is_us_option_contract_code;

#[test]
fn option_contract_detection_requires_a_strike_suffix() {
    assert!(!is_us_option_contract_code("AAPL260918C"));
    assert!(!is_us_option_contract_code("AAPL260918P"));
    assert!(is_us_option_contract_code("AAPL260918C00100000"));
    assert!(is_us_option_contract_code("AAPL260918P100"));
}
