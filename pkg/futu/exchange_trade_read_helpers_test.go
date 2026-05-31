package futu

import "testing"

func TestNormalizeTradeFilterTimeInput(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ISO8601 UTC with millis",
			input: "2026-05-31T12:34:56.789Z",
			want:  "2026-05-31 12:34:56",
		},
		{
			name:  "Space separated format",
			input: "2026-05-31 12:34:56",
			want:  "2026-05-31 12:34:56",
		},
		{
			name:  "Unknown format preserved",
			input: "2026/05/31 12:34:56",
			want:  "2026/05/31 12:34:56",
		},
		{
			name:  "Empty string",
			input: "   ",
			want:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeTradeFilterTimeInput(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeTradeFilterTimeInput(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBrokerTradeFilterConditionsNormalizesTimes(t *testing.T) {
	filter := brokerTradeFilterConditions("HK.00700", "2026-05-31T01:02:03.123Z", "2026-05-31T04:05:06.789Z", 1)

	if got := filter.GetBeginTime(); got != "2026-05-31 01:02:03" {
		t.Fatalf("BeginTime = %q, want 2026-05-31 01:02:03", got)
	}
	if got := filter.GetEndTime(); got != "2026-05-31 04:05:06" {
		t.Fatalf("EndTime = %q, want 2026-05-31 04:05:06", got)
	}
	if got := filter.GetCodeList(); len(got) != 1 || got[0] != "HK.00700" {
		t.Fatalf("CodeList = %#v, want [HK.00700]", got)
	}
	if got := filter.GetFilterMarket(); got != 1 {
		t.Fatalf("FilterMarket = %d, want 1", got)
	}
}
