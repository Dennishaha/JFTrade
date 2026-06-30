package pineworker

import (
	"fmt"
	"time"
)

type PerformanceGate struct {
	MaxDuration       time.Duration
	MaxDurationPerBar time.Duration
	MinCandlesPerSec  float64
	MaxRequestBytes   int
	MaxResponseBytes  int
	MaxPeakRSSBytes   int64
}

type PerformanceSample struct {
	Candles       int
	Duration      time.Duration
	RequestBytes  int
	ResponseBytes int
	PeakRSSBytes  int64
}

func DefaultPerformanceGate() PerformanceGate {
	return PerformanceGate{}
}

func CheckPerformanceGate(sample PerformanceSample, gate PerformanceGate) error {
	if sample.Candles <= 0 {
		return fmt.Errorf("candles must be positive")
	}
	if sample.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if gate.MaxDuration > 0 && sample.Duration > gate.MaxDuration {
		return fmt.Errorf("duration %s exceeds gate %s", sample.Duration, gate.MaxDuration)
	}
	if gate.MaxDurationPerBar > 0 && sample.Duration/time.Duration(sample.Candles) > gate.MaxDurationPerBar {
		return fmt.Errorf("duration per candle %s exceeds gate %s", sample.Duration/time.Duration(sample.Candles), gate.MaxDurationPerBar)
	}
	if gate.MinCandlesPerSec > 0 && CandlesPerSecond(sample) < gate.MinCandlesPerSec {
		return fmt.Errorf("throughput %.2f candles/sec below gate %.2f", CandlesPerSecond(sample), gate.MinCandlesPerSec)
	}
	if gate.MaxRequestBytes > 0 && sample.RequestBytes > gate.MaxRequestBytes {
		return fmt.Errorf("request bytes %d exceeds gate %d", sample.RequestBytes, gate.MaxRequestBytes)
	}
	if gate.MaxResponseBytes > 0 && sample.ResponseBytes > gate.MaxResponseBytes {
		return fmt.Errorf("response bytes %d exceeds gate %d", sample.ResponseBytes, gate.MaxResponseBytes)
	}
	if gate.MaxPeakRSSBytes > 0 && sample.PeakRSSBytes > gate.MaxPeakRSSBytes {
		return fmt.Errorf("peak RSS bytes %d exceeds gate %d", sample.PeakRSSBytes, gate.MaxPeakRSSBytes)
	}
	return nil
}

func CandlesPerSecond(sample PerformanceSample) float64 {
	if sample.Duration <= 0 {
		return 0
	}
	return float64(sample.Candles) / sample.Duration.Seconds()
}
