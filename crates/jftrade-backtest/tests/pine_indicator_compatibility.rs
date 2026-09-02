use jftrade_backtest::{pine_compatible_ema, pine_compatible_macd};

#[test]
fn pine_ema_seeds_from_sma_after_nan_warmup() {
    let values = pine_compatible_ema(&[1.0, 2.0, 3.0, 4.0, 5.0], 3).expect("EMA");

    assert!(values[..2].iter().all(|value| value.is_nan()));
    assert_eq!(&values[2..], &[2.0, 3.0, 4.0]);
}

#[test]
fn pine_ema_skips_nan_inputs_when_initializing_and_updating() {
    let values = pine_compatible_ema(&[1.0, f64::NAN, 3.0, 4.0, f64::NAN, 7.0], 3).expect("EMA");

    assert!(values[0].is_nan());
    assert!(values[1].is_nan());
    assert!(values[2].is_nan());
    assert_eq!(values[3], 2.6666666667);
    assert!(values[4].is_nan());
    assert_eq!(values[5], 4.8333333333);
}

#[test]
fn pine_macd_delays_signal_until_valid_macd_values_exist() {
    let (macd, signal, histogram) =
        pine_compatible_macd(&[1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0], 3, 5, 2).expect("MACD");

    assert!(macd[..4].iter().all(|value| value.is_nan()));
    assert!(signal[..5].iter().all(|value| value.is_nan()));
    assert!(histogram[..5].iter().all(|value| value.is_nan()));
    assert_eq!(macd[4], 1.0);
    assert_eq!(macd[5], 1.0);
    assert_eq!(macd[6], 1.0);
    assert_eq!(signal[5], 1.0);
    assert_eq!(signal[6], 1.0);
    assert_eq!(histogram[5], 0.0);
    assert_eq!(histogram[6], 0.0);
}

#[test]
fn pine_indicators_reject_zero_periods_without_partial_results() {
    assert!(pine_compatible_ema(&[1.0, 2.0], 0).is_err());
    assert!(pine_compatible_macd(&[1.0, 2.0], 0, 3, 2).is_err());
    assert!(pine_compatible_macd(&[1.0, 2.0], 3, 0, 2).is_err());
    assert!(pine_compatible_macd(&[1.0, 2.0], 3, 5, 0).is_err());
}

#[test]
fn pine_ema_returns_nan_warmup_when_period_exceeds_available_values() {
    let values = pine_compatible_ema(&[1.0, 2.0, 3.0], 4).expect("EMA");

    assert_eq!(values.len(), 3);
    assert!(values.iter().all(|value| value.is_nan()));
    assert!(pine_compatible_ema(&[], 1).expect("empty EMA").is_empty());
}

#[test]
fn pine_ema_preserves_unrounded_state_between_precision_outputs() {
    let values = pine_compatible_ema(
        &[
            0.9112069117230575,
            0.1645551720407894,
            -0.9644542670621914,
            0.3861207273438858,
            -0.3917016115977159,
            -0.6335474080768055,
            -0.5502087990448776,
            -0.8983848154715524,
            0.12542324640825342,
            0.8886820238110034,
        ],
        3,
    )
    .expect("EMA");

    assert_eq!(values[4], -0.0900449726);
    assert_eq!(values[9], 0.3063984097);
}

#[test]
fn pine_ema_matches_javascript_rounding_for_negative_ties() {
    let values = pine_compatible_ema(
        &[-0.00000000015, -0.00000000005, 0.00000000005, 0.00000000015],
        1,
    )
    .expect("EMA");

    assert_eq!(values[0], -0.0000000001);
    assert_eq!(values[1], 0.0);
    assert!(values[1].is_sign_negative());
    assert_eq!(values[2], 0.0000000001);
    assert_eq!(values[3], 0.0000000002);
}

#[test]
fn pine_ema_keeps_nan_and_infinity_values_unmodified() {
    let nan = pine_compatible_ema(&[f64::NAN], 1).expect("NaN EMA");
    let positive_infinity = pine_compatible_ema(&[f64::INFINITY], 1).expect("infinity EMA");
    let negative_infinity = pine_compatible_ema(&[f64::NEG_INFINITY], 1).expect("infinity EMA");

    assert!(nan[0].is_nan());
    assert_eq!(positive_infinity[0], f64::INFINITY);
    assert_eq!(negative_infinity[0], f64::NEG_INFINITY);
}

#[test]
fn pine_macd_rounds_signal_and_histogram_like_pinets() {
    let (macd, signal, histogram) = pine_compatible_macd(
        &[
            -0.025760421147209556,
            -0.44318445173295395,
            -0.44002735164640217,
            0.9469051423557706,
            0.10795358992379711,
            0.3137111482856676,
            -0.7343954966377952,
            0.21102526674998323,
            0.23450879473770003,
            0.6916279667265683,
        ],
        2,
        5,
        3,
    )
    .expect("MACD");

    assert_eq!(macd[9], 0.2483868862);
    assert_eq!(signal[9], 0.1628553388);
    assert_eq!(histogram[9], 0.0855315474);
}
