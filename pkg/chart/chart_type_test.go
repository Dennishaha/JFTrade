package chart

import "testing"

func TestNormalizeChartType(t *testing.T) {
	for _, test := range []struct {
		value string
		want  ChartType
	}{
		{value: "", want: ChartTypeStandard},
		{value: "standard", want: ChartTypeStandard},
		{value: "  HEIKINASHI ", want: ChartTypeHeikinAshi},
		{value: "renko", want: ChartTypeStandard},
	} {
		if got := NormalizeChartType(test.value); got != test.want {
			t.Errorf("NormalizeChartType(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}
