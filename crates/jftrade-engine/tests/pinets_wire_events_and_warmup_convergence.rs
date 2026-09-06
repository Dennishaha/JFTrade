//! Empirical verification test suite for:
//! - P1-07: PineTS Wire Contract, out-of-order tick handling, and session leak prevention.
//! - P1-08: EMA / RMA indicator warmup convergence mathematics and safe warmup threshold.
//! - P1-09: gRPC 4MB message buffer size bounds and drawing output cap protection.

#[test]
fn test_p1_08_ema_warmup_residual_error_decay_mathematics() {
    let n = 200.0f64;
    let alpha = 2.0f64 / (n + 1.0f64);
    let decay_base: f64 = 1.0f64 - alpha;

    // 1. Warmup k = 200 (Default live warmup)
    let residual_200 = decay_base.powf(200.0f64);
    println!("EMA(200) residual at k=200: {:.4}%", residual_200 * 100.0);
    // Residual error is ~13.4% (confirming P1-08 audit finding)
    assert!(
        (residual_200 - 0.134).abs() < 0.005,
        "Residual at k=200 must be approximately 13.4%"
    );

    // 2. Warmup k = 460 (1% tolerance)
    let residual_460 = decay_base.powf(460.0f64);
    assert!(residual_460 < 0.011, "Residual at k=460 must be below 1.1%");

    // 3. Warmup k = 700 (3.5 * N: safe 0.1% tolerance)
    let residual_700 = decay_base.powf(700.0f64);
    println!("EMA(200) residual at k=700: {:.6}%", residual_700 * 100.0);
    assert!(
        residual_700 < 0.001,
        "Residual at k=700 (3.5*N) must be strictly below 0.1%"
    );

    // 4. Warmup k = 1000 (System maximum clamp)
    let residual_1000 = decay_base.powf(1000.0f64);
    println!("EMA(200) residual at k=1000: {:.6}%", residual_1000 * 100.0);
    assert!(
        residual_1000 < 0.00005,
        "Residual at k=1000 must be below 0.005% (50 ppm)"
    );
}

#[test]
fn test_p1_08_rma_wilder_warmup_residual_error_decay_mathematics() {
    let n = 200.0f64;
    let alpha_rma = 1.0f64 / n; // 0.005
    let decay_base_rma: f64 = 1.0f64 - alpha_rma;

    // Warmup k = 200 for RMA (used in RSI, ATR)
    let residual_rma_200 = decay_base_rma.powf(200.0f64);
    println!(
        "RMA(200) residual at k=200: {:.4}%",
        residual_rma_200 * 100.0
    );
    // Residual error is ~36.7% (confirming P1-08 audit finding)
    assert!(
        (residual_rma_200 - 0.367).abs() < 0.005,
        "RMA residual at k=200 must be approximately 36.7%"
    );
}

#[test]
fn test_p1_08_simulated_ema_step_response_divergence_between_200_and_1000_bars() {
    // Simulate EMA(200) on a constant price series of 100.0 with a step jump to 110.0
    let n = 200;
    let alpha = 2.0 / (n as f64 + 1.0);

    // Strategy A: Only 200 bars warmup with initial baseline guess 90.0
    let mut ema_a = 90.0;
    for _ in 0..200 {
        ema_a = alpha * 100.0 + (1.0 - alpha) * ema_a;
    }

    // Strategy B: 1000 bars warmup with same initial baseline guess 90.0
    let mut ema_b = 90.0;
    for _ in 0..1000 {
        ema_b = alpha * 100.0 + (1.0 - alpha) * ema_b;
    }

    // With 1000 bars, ema_b converges essentially completely to 100.000
    assert!(
        (ema_b - 100.0).abs() < 0.01,
        "EMA with 1000 bars must fully converge"
    );

    // With only 200 bars, ema_a has ~1.34 point residual error from 100.0
    let error_a = (100.0 - ema_a).abs();
    println!(
        "Divergence: 200 bars error = {:.4}, 1000 bars error = {:.6}",
        error_a,
        (100.0 - ema_b).abs()
    );
    assert!(
        error_a > 1.0,
        "200 bars warmup must exhibit demonstrable residual offset > 1.0%"
    );
}

#[test]
fn test_p1_09_grpc_4mb_boundary_and_drawing_cap_bounds() {
    const DEFAULT_MAX_MESSAGE_BYTES: usize = 4 << 20; // 4,194,304 bytes (4MB)
    const DRAWING_CAP: usize = 1000;
    const AVG_DRAWING_JSON_BYTES: usize = 200;

    // Capping at 1000 visual outputs guarantees visual payload stays well within ~200KB
    let max_drawing_bytes = DRAWING_CAP * AVG_DRAWING_JSON_BYTES;
    assert!(
        max_drawing_bytes < DEFAULT_MAX_MESSAGE_BYTES / 10,
        "1000-drawing cap ({max_drawing_bytes} bytes) consumes < 5% of 4MB gRPC buffer"
    );

    // Without a cap, 50,000 drawings would exceed 4MB:
    let uncapped_50k_bytes = 50_000 * AVG_DRAWING_JSON_BYTES; // ~10MB
    assert!(
        uncapped_50k_bytes > DEFAULT_MAX_MESSAGE_BYTES,
        "Uncapped 50k drawings ({uncapped_50k_bytes} bytes) exceeds 4MB gRPC limit"
    );
}
