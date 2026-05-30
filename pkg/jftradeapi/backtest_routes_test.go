package jftradeapi

import (
	"reflect"
	"testing"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
)

func TestNormalizeBacktestSyncSessionScope(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "regular passthrough", input: "regular", want: "regular"},
		{name: "extended passthrough", input: "extended", want: "extended"},
		{name: "case insensitive", input: " ExTeNdEd ", want: "extended"},
		{name: "unknown falls back to legacy", input: "all", want: "legacy"},
		{name: "empty falls back to legacy", input: "", want: "legacy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBacktestSyncSessionScope(tt.input); got != tt.want {
				t.Fatalf("normalizeBacktestSyncSessionScope(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPlanBacktestSyncIntervals(t *testing.T) {
	tests := []struct {
		name         string
		symbol       string
		sessionScope string
		requested    []bbgotypes.Interval
		want         []bbgotypes.Interval
	}{
		{
			name:         "us extended high periods collapse to one hour base",
			symbol:       "US.AAPL",
			sessionScope: "extended",
			requested: []bbgotypes.Interval{
				bbgotypes.Interval1d,
				bbgotypes.Interval1w,
				bbgotypes.Interval1mo,
				bbgotypes.Interval2h,
				bbgotypes.Interval1h,
			},
			want: []bbgotypes.Interval{bbgotypes.Interval1h},
		},
		{
			name:         "hk regular keeps daily but rewrites higher intraday",
			symbol:       "HK.00700",
			sessionScope: "regular",
			requested: []bbgotypes.Interval{
				bbgotypes.Interval1d,
				bbgotypes.Interval4h,
				bbgotypes.Interval1h,
			},
			want: []bbgotypes.Interval{bbgotypes.Interval1d, bbgotypes.Interval1h},
		},
		{
			name:         "three day and two week collapse to one day",
			symbol:       "US.AAPL",
			sessionScope: "legacy",
			requested: []bbgotypes.Interval{
				bbgotypes.Interval("3d"),
				bbgotypes.Interval("2w"),
				bbgotypes.Interval1d,
			},
			want: []bbgotypes.Interval{bbgotypes.Interval1d},
		},
		{
			name:         "dedupe keeps first planned base interval order",
			symbol:       "HK.00700",
			sessionScope: "regular",
			requested: []bbgotypes.Interval{
				bbgotypes.Interval1h,
				bbgotypes.Interval4h,
				bbgotypes.Interval1d,
				bbgotypes.Interval2h,
			},
			want: []bbgotypes.Interval{bbgotypes.Interval1h, bbgotypes.Interval1d},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planBacktestSyncIntervals(tt.symbol, tt.requested, tt.sessionScope)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("planBacktestSyncIntervals(%s, %v, %s) = %v, want %v", tt.symbol, tt.requested, tt.sessionScope, got, tt.want)
			}
		})
	}
}
