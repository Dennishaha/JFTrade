package ir

import "testing"

func TestParseIndicatorBindingNormalizesSourceAwareIndicators(t *testing.T) {
	tests := []struct {
		name     string
		stmt     *LetStmt
		wantKey  string
		wantKind string
		wantArgs []string
	}{
		{
			name:     "macd with timeframe and source",
			stmt:     &LetStmt{Range: SourceRange{StartLine: 1}, Name: "signal", Expression: "macd(12, 26, 9, day, hlc3)"},
			wantKey:  "macd:hlc3:12:26:9:day",
			wantKind: "macd",
			wantArgs: []string{"12", "26", "9", "day", "hlc3"},
		},
		{
			name:     "bollinger with timeframe and source",
			stmt:     &LetStmt{Range: SourceRange{StartLine: 2}, Name: "band", Expression: "bollinger(20, 2, week, ohlc4)"},
			wantKey:  "bollinger:ohlc4:20:2:week",
			wantKind: "bollinger",
			wantArgs: []string{"20", "2", "week", "ohlc4"},
		},
		{
			name:     "stoch with explicit timeframe",
			stmt:     &LetStmt{Range: SourceRange{StartLine: 3}, Name: "osc", Expression: "stoch(hlc3, high, low, 14, day)"},
			wantKey:  "stoch:hlc3:14:day",
			wantKind: "stoch",
			wantArgs: []string{"hlc3", "14", "day"},
		},
		{
			name:     "security source with lookback",
			stmt:     &LetStmt{Range: SourceRange{StartLine: 4}, Name: "dailyClose", Expression: "security_source(close, week, 2)"},
			wantKey:  "security_source:week:close:2",
			wantKind: "security_source",
			wantArgs: []string{"close", "week", "2"},
		},
		{
			name:     "cum volume",
			stmt:     &LetStmt{Range: SourceRange{StartLine: 5}, Name: "vol", Expression: "cum(volume)"},
			wantKey:  "cum:volume",
			wantKind: "cum",
			wantArgs: []string{"volume"},
		},
		{
			name:     "supertrend with timeframe",
			stmt:     &LetStmt{Range: SourceRange{StartLine: 6}, Name: "trend", Expression: "supertrend(3, 10, day)"},
			wantKey:  "supertrend:3:10:day",
			wantKind: "supertrend",
			wantArgs: []string{"3", "10", "day"},
		},
		{
			name:     "obv uses default close source",
			stmt:     &LetStmt{Range: SourceRange{StartLine: 7}, Name: "obvValue", Expression: "obv()"},
			wantKey:  "obv:close",
			wantKind: "obv",
			wantArgs: []string{"close"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binding, recognized, err := parseIndicatorBinding(tc.stmt)
			if err != nil {
				t.Fatalf("parseIndicatorBinding() error = %v", err)
			}
			if !recognized {
				t.Fatal("parseIndicatorBinding() recognized = false, want true")
			}
			if binding.Kind != tc.wantKind || binding.Key != tc.wantKey {
				t.Fatalf("binding = %#v, want kind=%q key=%q", binding, tc.wantKind, tc.wantKey)
			}
			if len(binding.Args) != len(tc.wantArgs) {
				t.Fatalf("binding.Args len = %d, want %d (%#v)", len(binding.Args), len(tc.wantArgs), binding.Args)
			}
			for index, want := range tc.wantArgs {
				if binding.Args[index] != want {
					t.Fatalf("binding.Args[%d] = %q, want %q; all=%#v", index, binding.Args[index], want, binding.Args)
				}
			}
		})
	}
}

func TestParseIndicatorBindingRejectsInvalidIndicatorParameters(t *testing.T) {
	tests := []struct {
		name string
		stmt *LetStmt
	}{
		{
			name: "macd rejects invalid timeframe",
			stmt: &LetStmt{Range: SourceRange{StartLine: 21}, Name: "signal", Expression: "macd(12, 26, 9, noon, close)"},
		},
		{
			name: "stoch requires literal high low",
			stmt: &LetStmt{Range: SourceRange{StartLine: 22}, Name: "osc", Expression: "stoch(close, close, low, 14)"},
		},
		{
			name: "security source rejects negative lookback",
			stmt: &LetStmt{Range: SourceRange{StartLine: 23}, Name: "daily", Expression: "security_source(close, day, -1)"},
		},
		{
			name: "bollinger rejects unsupported source",
			stmt: &LetStmt{Range: SourceRange{StartLine: 24}, Name: "band", Expression: "bollinger(20, 2, day, spread)"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, recognized, err := parseIndicatorBinding(tc.stmt); err == nil {
				t.Fatalf("parseIndicatorBinding(%q) error = nil, want validation error", tc.stmt.Expression)
			} else if recognized {
				t.Fatalf("parseIndicatorBinding(%q) recognized = true on validation error", tc.stmt.Expression)
			}
		})
	}
}

func TestBuildDivergenceRequirementKeySupportsExpectedIndicators(t *testing.T) {
	tests := []struct {
		name    string
		binding plannedBinding
		wantKey string
		wantOK  bool
	}{
		{
			name:    "macd divergence key",
			binding: plannedBinding{Kind: "macd", Args: []string{"12", "26", "9"}},
			wantKey: "divergence:macd:12:26:9:top:5",
			wantOK:  true,
		},
		{
			name:    "kdj divergence key",
			binding: plannedBinding{Kind: "kdj", Args: []string{"9", "3", "3"}},
			wantKey: "divergence:kdj:9:3:3:top:5",
			wantOK:  true,
		},
		{
			name:    "unsupported indicator",
			binding: plannedBinding{Kind: "bollinger", Args: []string{"20", "2"}},
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := buildDivergenceRequirementKey(tc.binding, "top", 5)
			if ok != tc.wantOK {
				t.Fatalf("buildDivergenceRequirementKey() ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.wantKey {
				t.Fatalf("buildDivergenceRequirementKey() = %q, want %q", got, tc.wantKey)
			}
		})
	}
}
