package pine

import "testing"

func TestExtendedTickerRequestSecuritySupportsCurrentSymbolOnly(t *testing.T) {
	valid := []string{
		"syminfo.tickerid",
		"ticker.heikinashi(syminfo.tickerid)",
		"ticker.standard(syminfo.tickerid)",
		"ticker.standard()",
		"ticker.inherit(ticker.heikinashi(syminfo.tickerid), syminfo.tickerid)",
	}
	for _, ticker := range valid {
		t.Run(ticker, func(t *testing.T) {
			if !supportedRequestSecurityTicker(ticker) {
				t.Fatalf("supportedRequestSecurityTicker(%q) = false", ticker)
			}
			lowered, ok := lowerSupportedRequestSecurity([]string{ticker, `"60"`, "close"})
			if !ok || lowered != "security_source(close, hour)" {
				t.Fatalf("lowerSupportedRequestSecurity(%q) = %q/%v", ticker, lowered, ok)
			}
			line := parsedLine{number: 7, trimmed: `value = request.security(` + ticker + `, "60", close)`}
			if diagnostic, rejected := requestSecurityUnsupportedDiagnostic(line); rejected {
				t.Fatalf("requestSecurityUnsupportedDiagnostic(%q) = %+v", ticker, diagnostic)
			}
		})
	}

	for _, ticker := range []string{
		`"NASDAQ:AAPL"`,
		"ticker.heikinashi(\"NASDAQ:AAPL\")",
		"ticker.standard(otherTicker)",
		"ticker.inherit(syminfo.tickerid, \"NASDAQ:AAPL\")",
		"ticker.renko(syminfo.tickerid)",
	} {
		t.Run("reject "+ticker, func(t *testing.T) {
			if supportedRequestSecurityTicker(ticker) {
				t.Fatalf("supportedRequestSecurityTicker(%q) = true", ticker)
			}
			line := parsedLine{number: 9, trimmed: `value = request.security(` + ticker + `, "60", close)`}
			diagnostic, rejected := requestSecurityUnsupportedDiagnostic(line)
			if !rejected || diagnostic.Code != "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL" {
				t.Fatalf("requestSecurityUnsupportedDiagnostic(%q) = %+v/%v", ticker, diagnostic, rejected)
			}
		})
	}
}

func TestCompileAcceptsExtendedTickerAndChartFlags(t *testing.T) {
	script := `//@version=6
strategy("Extended ticker")
haClose = request.security(ticker.heikinashi(syminfo.tickerid), "60", close)
standardClose = request.security(ticker.standard(), "60", close)
signal = chart.is_heikinashi ? haClose > standardClose : chart.is_standard
if signal
    strategy.entry("long", strategy.long)`
	compilation, err := Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if compilation.Program == nil || len(compilation.Program.Hooks) == 0 || len(compilation.Program.Hooks[0].Statements) == 0 {
		t.Fatalf("Compile() program = %#v", compilation.Program)
	}
	if len(compilation.Requirements.Indicators) == 0 {
		t.Fatalf("Compile() did not retain indicator requirements: %#v", compilation.Requirements)
	}
}
